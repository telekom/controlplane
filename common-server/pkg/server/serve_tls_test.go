// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	certutil "k8s.io/client-go/util/cert"

	"github.com/telekom/controlplane/common-server/pkg/server/serve"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServeTLS", func() {
	It("serves TLS on a pre-bound listener", func() {
		certPEM, keyPEM, err := certutil.GenerateSelfSignedCertKey("localhost", []net.IP{net.ParseIP("127.0.0.1")}, nil)
		Expect(err).NotTo(HaveOccurred())

		tempDir := GinkgoT().TempDir()
		certFile := filepath.Join(tempDir, "tls.crt")
		keyFile := filepath.Join(tempDir, "tls.key")
		Expect(os.WriteFile(certFile, certPEM, 0o600)).To(Succeed())
		Expect(os.WriteFile(keyFile, keyPEM, 0o600)).To(Succeed())

		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- serve.ServeTLS(ctx, app, listener, certFile, keyFile)
		}()

		certPool := x509.NewCertPool()
		Expect(certPool.AppendCertsFromPEM(certPEM)).To(BeTrue())
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS13,
			RootCAs:    certPool,
			ServerName: "localhost",
		}
		dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
		Eventually(func() error {
			conn, dialErr := tls.DialWithDialer(dialer, "tcp4", listener.Addr().String(), tlsConfig)
			if dialErr != nil {
				return dialErr
			}
			return conn.Close()
		}, time.Second, 10*time.Millisecond).Should(Succeed())

		Expect(app.Shutdown()).To(Succeed())
		Eventually(done, time.Second, 10*time.Millisecond).Should(Receive(Succeed()))
	})

	It("returns certificate loading errors", func() {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer listener.Close() //nolint:errcheck // The listener is only test setup cleanup.

		err = serve.ServeTLS(context.Background(), fiber.New(), listener, "/missing/tls.crt", "/missing/tls.key")
		Expect(err).To(HaveOccurred())
	})
})
