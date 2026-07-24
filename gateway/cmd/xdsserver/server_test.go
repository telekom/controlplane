// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package xdsserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	relayURI = "spiffe://tardis.telekom.de/relay/one"
	nodeID   = "gateway-node-one"
	typeURL  = resourcev3.ClusterType
)

var _ = Describe("secure xDS server", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		cache      cachev3.SnapshotCache
		server     *Server
		registry   *prometheus.Registry
		serverPool *x509.CertPool
		clientCert tls.Certificate
		noURICert  tls.Certificate
		wrongCert  tls.Certificate
	)

	BeforeEach(func() {
		// arrange
		ctx, cancel = context.WithCancel(context.Background())
		cache = cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		registry = prometheus.NewRegistry()
		dir := GinkgoT().TempDir()
		caCert, caKey, caPEM := createCA()
		serverCert, serverKey := createCertificate(caCert, caKey, "xds-server", nil, []string{"localhost"}, true)
		clientPEM, clientKey := createCertificate(caCert, caKey, "relay", []string{relayURI}, nil, false)
		noURIPEM, noURIKey := createCertificate(caCert, caKey, "relay-without-uri", nil, nil, false)
		wrongCA, wrongCAKey, _ := createCA()
		wrongPEM, wrongKey := createCertificate(wrongCA, wrongCAKey, "wrong-relay", []string{relayURI}, nil, false)

		serverCertFile := writeFile(dir, "server.crt", serverCert)
		serverKeyFile := writeFile(dir, "server.key", serverKey)
		caFile := writeFile(dir, "client-ca.crt", caPEM)
		assignmentsFile := writeFile(dir, "assignments.yaml", []byte(relayURI+":\n  - "+nodeID+"\n"))
		var err error
		clientCert, err = tls.X509KeyPair(clientPEM, clientKey)
		Expect(err).NotTo(HaveOccurred())
		noURICert, err = tls.X509KeyPair(noURIPEM, noURIKey)
		Expect(err).NotTo(HaveOccurred())
		wrongCert, err = tls.X509KeyPair(wrongPEM, wrongKey)
		Expect(err).NotTo(HaveOccurred())
		serverPool = x509.NewCertPool()
		Expect(serverPool.AppendCertsFromPEM(serverCert)).To(BeTrue())

		server, err = Start(ctx, cache, Config{
			Address: "127.0.0.1:0", ServerCertificateFile: serverCertFile, ServerKeyFile: serverKeyFile,
			ClientCAFile: caFile, RelayAssignmentsFile: assignmentsFile, Registry: registry,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
	})

	It("accepts an assigned relay and records request, response, ACK, and stream metrics", func() {
		// arrange
		snapshot, err := cachev3.NewSnapshot("1", map[resourcev3.Type][]cachetypes.Resource{resourcev3.ClusterType: {}})
		Expect(err).NotTo(HaveOccurred())
		Expect(cache.SetSnapshot(ctx, nodeID, snapshot)).To(Succeed())
		conn := dial(server, serverPool, &clientCert)
		defer conn.Close()
		stream, err := discovery.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
		Expect(err).NotTo(HaveOccurred())

		// act
		Expect(stream.Send(request(nodeID, "", nil))).To(Succeed())
		response, err := stream.Recv()

		// assert
		Expect(err).NotTo(HaveOccurred())
		Expect(response.GetVersionInfo()).To(Equal("1"))
		ack := request(nodeID, response.GetNonce(), nil)
		ack.VersionInfo = response.GetVersionInfo()
		Expect(stream.Send(ack)).To(Succeed())
		Eventually(func() float64 { return metricValue(registry, "gateway_xds_acks_total", typeURL) }).Should(Equal(float64(1)))
		Expect(metricValue(registry, "gateway_xds_requests_total", typeURL)).To(Equal(float64(2)))
		Expect(metricValue(registry, "gateway_xds_responses_total", typeURL)).To(Equal(float64(1)))
		Expect(metricValue(registry, "gateway_xds_active_streams", "ads")).To(Equal(float64(1)))
		Expect(stream.CloseSend()).To(Succeed())
		Eventually(func() float64 {
			return metricValue(registry, "gateway_xds_active_streams", "ads")
		}).Should(Equal(float64(0)))
	})

	It("rejects a client certificate signed by an untrusted CA", func() {
		// act
		conn, err := grpc.NewClient(server.Address().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS(serverPool, &wrongCert))))
		Expect(err).NotTo(HaveOccurred())
		defer conn.Close()
		stream, err := discovery.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
		if err == nil {
			err = stream.Send(request(nodeID, "", nil))
		}
		if err == nil {
			_, err = stream.Recv()
		}

		// assert
		Expect(err).To(HaveOccurred())
	})

	It("rejects a client without a certificate", func() {
		// act
		conn, err := grpc.NewClient(server.Address().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS(serverPool, nil))))
		Expect(err).NotTo(HaveOccurred())
		defer conn.Close()
		stream, err := discovery.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
		if err == nil {
			err = stream.Send(request(nodeID, "", nil))
		}
		if err == nil {
			_, err = stream.Recv()
		}

		// assert
		Expect(err).To(HaveOccurred())
	})

	It("rejects a verified client without exactly one URI SAN using a generic error", func() {
		// arrange
		conn := dial(server, serverPool, &noURICert)
		defer conn.Close()
		stream, err := discovery.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
		Expect(err).NotTo(HaveOccurred())

		// act
		Expect(stream.Send(request(nodeID, "", nil))).To(Succeed())
		_, err = stream.Recv()

		// assert
		Expect(status.Code(err)).To(Equal(codes.PermissionDenied))
		Expect(status.Convert(err).Message()).To(Equal(unauthorizedMessage))
	})

	It("rejects an unassigned node before cache access", func() {
		// arrange
		conn := dial(server, serverPool, &clientCert)
		defer conn.Close()
		stream, err := discovery.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
		Expect(err).NotTo(HaveOccurred())

		// act
		Expect(stream.Send(request("unassigned", "", nil))).To(Succeed())
		_, err = stream.Recv()

		// assert
		Expect(status.Code(err)).To(Equal(codes.PermissionDenied))
		Expect(status.Convert(err).Message()).To(Equal(unauthorizedMessage))
		Expect(metricValue(registry, "gateway_xds_unauthorized_requests_total", typeURL)).To(Equal(float64(1)))
	})

	It("rejects a node ID changed on an established stream", func() {
		// arrange
		snapshot, err := cachev3.NewSnapshot("1", map[resourcev3.Type][]cachetypes.Resource{resourcev3.ClusterType: {}})
		Expect(err).NotTo(HaveOccurred())
		Expect(cache.SetSnapshot(ctx, nodeID, snapshot)).To(Succeed())
		conn := dial(server, serverPool, &clientCert)
		defer conn.Close()
		stream, err := discovery.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stream.Send(request(nodeID, "", nil))).To(Succeed())
		response, err := stream.Recv()
		Expect(err).NotTo(HaveOccurred())

		// act
		Expect(stream.Send(request("changed-node", response.GetNonce(), nil))).To(Succeed())
		_, err = stream.Recv()

		// assert
		Expect(status.Code(err)).To(Equal(codes.PermissionDenied))
		Eventually(func() float64 {
			return metricValue(registry, "gateway_xds_active_streams", "ads")
		}).Should(Equal(float64(0)))
	})

	It("accepts ACKs that omit the bound node", func() {
		// arrange
		callbackRegistry := prometheus.NewRegistry()
		callbacks, err := newCallbacks(true, relayAssignments{relayURI: {nodeID: {}}}, callbackRegistry)
		Expect(err).NotTo(HaveOccurred())
		callbacks.streams[1] = streamIdentity{uri: relayURI, typeURL: "ads"}
		Expect(callbacks.OnStreamRequest(1, request(nodeID, "", nil))).To(Succeed())

		// act
		err = callbacks.OnStreamRequest(1, request("", "nonce", nil))

		// assert
		Expect(err).NotTo(HaveOccurred())
	})

	It("counts NACKs by type URL", func() {
		// arrange
		isolatedRegistry := prometheus.NewRegistry()
		callbacks, err := newCallbacks(false, nil, isolatedRegistry)
		Expect(err).NotTo(HaveOccurred())
		Expect(callbacks.OnStreamOpen(context.Background(), 1, "")).To(Succeed())

		// act
		Expect(callbacks.OnStreamRequest(1, request(nodeID, "nonce", status.New(codes.Internal, "rejected").Proto()))).To(Succeed())

		// assert
		Expect(metricValue(isolatedRegistry, "gateway_xds_nacks_total", typeURL)).To(Equal(float64(1)))
	})
})

var _ = Describe("configuration", func() {
	It("preserves plaintext mode when no security files are configured", func() {
		// arrange
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		registry := prometheus.NewRegistry()

		// act
		server, err := Start(ctx, cache, Config{Address: "127.0.0.1:0", Registry: registry})

		// assert
		Expect(err).NotTo(HaveOccurred())
		conn, err := grpc.NewClient(server.Address().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).NotTo(HaveOccurred())
		Expect(conn.Close()).To(Succeed())
	})

	It("fails closed when only some security files are configured", func() {
		// arrange
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)

		// act
		_, err := Start(context.Background(), cache, Config{Address: "127.0.0.1:0", ServerCertificateFile: "server.crt"})

		// assert
		Expect(err).To(MatchError(ContainSubstring("must all be configured")))
	})
})

var _ = Describe("TLS reload", func() {
	It("retains the last valid configuration after an invalid reload", func() {
		// arrange
		dir := GinkgoT().TempDir()
		caCert, caKey, caPEM := createCA()
		certificate, key := createCertificate(caCert, caKey, "xds-server", nil, []string{"localhost"}, true)
		certificateFile := writeFile(dir, "server.crt", certificate)
		keyFile := writeFile(dir, "server.key", key)
		caFile := writeFile(dir, "client-ca.crt", caPEM)
		reloader, err := newTLSReloader(certificateFile, keyFile, caFile)
		Expect(err).NotTo(HaveOccurred())
		valid := reloader.current

		// act
		Expect(os.WriteFile(caFile, []byte("not a CA"), 0o600)).To(Succeed())
		loaded, err := reloader.getConfigForClient(nil)

		// assert
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded).To(BeIdenticalTo(valid))
	})
})

func request(node, nonce string, detail *statuspb.Status) *discovery.DiscoveryRequest {
	req := &discovery.DiscoveryRequest{Node: &core.Node{Id: node}, TypeUrl: typeURL, ResponseNonce: nonce}
	if detail != nil {
		req.ErrorDetail = detail
	}
	return req
}

func dial(server *Server, roots *x509.CertPool, certificate *tls.Certificate) *grpc.ClientConn {
	conn, err := grpc.NewClient(server.Address().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS(roots, certificate))))
	Expect(err).NotTo(HaveOccurred())
	return conn
}

func clientTLS(roots *x509.CertPool, certificate *tls.Certificate) *tls.Config {
	config := &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12}
	if certificate != nil {
		config.Certificates = []tls.Certificate{*certificate}
	}
	return config
}

func metricValue(registry *prometheus.Registry, name, typeValue string) float64 {
	families, err := registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, sample := range family.GetMetric() {
			for _, label := range sample.GetLabel() {
				if label.GetName() == "type_url" && label.GetValue() == typeValue {
					if sample.Gauge != nil {
						return sample.GetGauge().GetValue()
					}
					return sample.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func createCA() (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "test-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	certificate, err := x509.ParseCertificate(der)
	Expect(err).NotTo(HaveOccurred())
	return certificate, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func createCertificate(ca *x509.Certificate, caKey *ecdsa.PrivateKey, commonName string, uriStrings, dnsNames []string, server bool) ([]byte, []byte) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	usage := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if server {
		usage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: commonName}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: usage, DNSNames: dnsNames}
	for _, rawURI := range uriStrings {
		parsed, err := url.Parse(rawURI)
		Expect(err).NotTo(HaveOccurred())
		template.URIs = append(template.URIs, parsed)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	Expect(err).NotTo(HaveOccurred())
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func writeFile(dir, name string, contents []byte) string {
	path := filepath.Join(dir, name)
	Expect(os.WriteFile(path, contents, 0o600)).To(Succeed())
	return path
}
