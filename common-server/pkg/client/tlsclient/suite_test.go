// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tlsclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTLSClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TLSClient Suite")
}

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(cn string) *testCA {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	cert, err := x509.ParseCertificate(der)
	Expect(err).NotTo(HaveOccurred())
	return &testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// serverCert issues a certificate for 127.0.0.1, or for dnsName only if set.
func (ca *testCA) serverCert(dnsName string) tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if dnsName != "" {
		tmpl.DNSNames = []string{dnsName}
	} else {
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	Expect(err).NotTo(HaveOccurred())
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func startTLSServer(cert tls.Certificate, h http.Handler) *httptest.Server {
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	DeferCleanup(srv.Close)
	return srv
}

type staticCA struct {
	mu    sync.Mutex
	pem   []byte
	err   error
	calls atomic.Int32
}

func (s *staticCA) Resolve(_ context.Context) ([]byte, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pem, s.err
}

func (s *staticCA) set(pem []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pem, s.err = pem, err
}

func join(pems ...[]byte) []byte {
	var out []byte
	for _, p := range pems {
		out = append(out, p...)
	}
	return out
}

// tokenServer is a configurable OAuth2 token endpoint.
type tokenServer struct {
	passwordCalls atomic.Int32
	refreshCalls  atomic.Int32
	ccCalls       atomic.Int32
	// passwordStatus/refreshStatus override the response status if != 0.
	passwordStatus atomic.Int32
	refreshStatus  atomic.Int32
	expiresIn      atomic.Int32
	block          chan struct{}
}

func (t *tokenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.block != nil {
		select {
		case <-t.block:
		case <-r.Context().Done():
			return
		}
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var status int32
	switch r.Form.Get("grant_type") {
	case "password":
		t.passwordCalls.Add(1)
		status = t.passwordStatus.Load()
	case "refresh_token":
		t.refreshCalls.Add(1)
		status = t.refreshStatus.Load()
	case "client_credentials":
		t.ccCalls.Add(1)
	}
	if status != 0 {
		w.WriteHeader(int(status))
		_, _ = w.Write([]byte(`{"error":"failed"}`))
		return
	}
	expires := int(t.expiresIn.Load())
	if expires == 0 {
		expires = 3600
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "token-" + r.Form.Get("grant_type"),
		"refresh_token": "refresh",
		"token_type":    "Bearer",
		"expires_in":    expires,
	})
}

// apiHandler records the Authorization header.
type apiHandler struct {
	mu   sync.Mutex
	auth []string
}

func (a *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.auth = append(a.auth, r.Header.Get("Authorization"))
	a.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (a *apiHandler) last() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.auth) == 0 {
		return ""
	}
	return a.auth[len(a.auth)-1]
}

func get(c *Client, url string) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	Expect(err).NotTo(HaveOccurred())
	resp, err := c.Doer().Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}
