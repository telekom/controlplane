// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

type testCert struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newCert(tmpl *x509.Certificate, parent *testCert) testCert {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())
	signer, parentCert := key, tmpl
	if parent != nil {
		signer, parentCert = parent.key, parent.cert
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parentCert, &key.PublicKey, signer)
	Expect(err).ToNot(HaveOccurred())
	c, err := x509.ParseCertificate(der)
	Expect(err).ToNot(HaveOccurred())
	return testCert{cert: c, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

var serial int64

func tmpl(cn string, isCA bool, hosts ...string) *x509.Certificate {
	serial++
	t := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	if isCA {
		t.KeyUsage |= x509.KeyUsageCertSign
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			t.IPAddresses = append(t.IPAddresses, ip)
		} else {
			t.DNSNames = append(t.DNSNames, h)
		}
	}
	return t
}

func serve(leaf testCert) *httptest.Server {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leaf.cert.Raw}, PrivateKey: leaf.key}}}
	srv.StartTLS()
	return srv
}

func get(b *ProviderCABundle, url string) error {
	tr, err := NewProviderTransport(nil, b)
	Expect(err).ToNot(HaveOccurred())
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: DenyRedirects}
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

var _ = Describe("ProviderCABundle", func() {
	var ca, leaf testCert

	BeforeEach(func() {
		SystemCertPool = func() (*x509.CertPool, error) { return x509.NewCertPool(), nil }
		ca = newCert(tmpl("ca", true), nil)
		leaf = newCert(tmpl("leaf", false, "127.0.0.1"), &ca)
	})
	AfterEach(func() { SystemCertPool = x509.SystemCertPool })

	Context("parsing", func() {
		It("accepts whitespace-separated blocks and deduplicates", func() {
			other := newCert(tmpl("ca2", true), nil)
			in := append(append(append([]byte("\n  "), ca.pem...), other.pem...), ca.pem...)
			b, err := ParseProviderCABundle(in)
			Expect(err).ToNot(HaveOccurred())
			Expect(b.Len()).To(Equal(2))
			b2, err := ParseProviderCABundle(append(other.pem, ca.pem...))
			Expect(err).ToNot(HaveOccurred())
			Expect(b2.Digest()).To(Equal(b.Digest()))
			Expect(b2.CanonicalPEM()).To(Equal(b.CanonicalPEM()))
		})

		DescribeTable("rejects invalid input",
			func(in func() []byte) {
				_, err := ParseProviderCABundle(in())
				Expect(err).To(HaveOccurred())
			},
			Entry("empty", func() []byte { return nil }),
			Entry("whitespace", func() []byte { return []byte(" \n") }),
			Entry("garbage prefix", func() []byte { return append([]byte("x"), ca.pem...) }),
			Entry("garbage suffix", func() []byte { return append(ca.pem, []byte("trailing")...) }),
			Entry("key block", func() []byte {
				return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte{1}})
			}),
			Entry("headers", func() []byte {
				return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Headers: map[string]string{"a": "b"}, Bytes: ca.cert.Raw})
			}),
			Entry("malformed DER", func() []byte {
				return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1, 2, 3}})
			}),
			Entry("truncated block", func() []byte { return ca.pem[:len(ca.pem)-10] }),
			Entry("too large", func() []byte {
				return append([]byte(strings.Repeat(" ", MaxProviderCABundleBytes)), ca.pem...)
			}),
			Entry("too many certs", func() []byte {
				out := make([]byte, 0, len(ca.pem)*(MaxProviderCABundleCerts+1))
				for range MaxProviderCABundleCerts + 1 {
					out = append(out, ca.pem...)
				}
				return out
			}),
			Entry("malformed cert then valid", func() []byte {
				return append([]byte("-----BEGIN CERTIFICATE-----\n!!!!\n-----END CERTIFICATE-----\n"), ca.pem...)
			}),
			Entry("malformed private key then valid", func() []byte {
				return append([]byte("-----BEGIN PRIVATE KEY-----\n!!!!\n-----END PRIVATE KEY-----\n"), ca.pem...)
			}),
			Entry("unterminated BEGIN then valid", func() []byte {
				return append([]byte("-----BEGIN CERTIFICATE-----\nAAAA\n"), ca.pem...)
			}),
			Entry("nested BEGIN inside block", func() []byte {
				return append(append([]byte("-----BEGIN CERTIFICATE-----\n"), ca.pem...), []byte("-----END CERTIFICATE-----\n")...)
			}),
			Entry("valid, invalid, valid", func() []byte {
				bad := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1, 2, 3}})
				return append(append(append([]byte{}, ca.pem...), bad...), ca.pem...)
			}),
			Entry("mismatched END", func() []byte {
				return []byte(strings.Replace(string(ca.pem), "END CERTIFICATE", "END X509 CRL", 1))
			}),
			Entry("junk glued to END", func() []byte { return []byte(strings.TrimRight(string(ca.pem), "\n") + "junk\n") }),
			Entry("interstitial garbage", func() []byte {
				return append(append(append([]byte{}, ca.pem...), []byte("# comment\n")...), ca.pem...)
			}),
			Entry("non-base64 body", func() []byte {
				return []byte(strings.Replace(string(ca.pem), "\n", "\n*", 1))
			}),
			Entry("non-self-signed leaf", func() []byte { return leaf.pem }),
			Entry("CA without CertSign", func() []byte {
				t := tmpl("bad-ca", true)
				t.KeyUsage = x509.KeyUsageDigitalSignature
				return newCert(t, nil).pem
			}),
		)

		It("accepts CRLF framing and counts deduplicated certificates", func() {
			other := newCert(tmpl("ca2", true), nil)
			crlf := []byte(strings.ReplaceAll(string(other.pem), "\n", "\r\n"))
			b, err := ParseProviderCABundle(append(append(append([]byte{}, ca.pem...), crlf...), ca.pem...))
			Expect(err).ToNot(HaveOccurred())
			Expect(b.Len()).To(Equal(2))
			ref, err := ParseProviderCABundle(append(append([]byte{}, other.pem...), ca.pem...))
			Expect(err).ToNot(HaveOccurred())
			Expect(b.Digest()).To(Equal(ref.Digest()))
		})

		It("counts duplicates against the certificate limit", func() {
			out := make([]byte, 0, len(ca.pem)*MaxProviderCABundleCerts)
			for range MaxProviderCABundleCerts {
				out = append(out, ca.pem...)
			}
			b, err := ParseProviderCABundle(out)
			Expect(err).ToNot(HaveOccurred())
			Expect(b.Len()).To(Equal(1))
		})

		Context("merging", func() {
			parse := func(in []byte) *ProviderCABundle {
				GinkgoHelper()
				b, err := ParseProviderCABundle(in)
				Expect(err).ToNot(HaveOccurred())
				return b
			}

			It("returns the deduplicated union in canonical order independent of input order", func() {
				other := newCert(tmpl("ca2", true), nil)
				third := newCert(tmpl("ca3", true), nil)
				a := parse(append(append([]byte{}, ca.pem...), other.pem...))
				b := parse(append(append([]byte{}, other.pem...), third.pem...))
				ab, err := MergeProviderCABundles(a, b)
				Expect(err).ToNot(HaveOccurred())
				ba, err := MergeProviderCABundles(b, a)
				Expect(err).ToNot(HaveOccurred())
				Expect(ab.Len()).To(Equal(3))
				Expect(ab.CanonicalPEM()).To(Equal(ba.CanonicalPEM()))
				all := parse(append(append(append([]byte{}, third.pem...), ca.pem...), other.pem...))
				Expect(ab.Digest()).To(Equal(all.Digest()))
			})

			It("enforces the certificate limit on the union", func() {
				var bundles []*ProviderCABundle
				for i := range MaxProviderCABundleCerts + 1 {
					bundles = append(bundles, parse(newCert(tmpl(fmt.Sprintf("ca-%d", i), true), nil).pem))
				}
				_, err := MergeProviderCABundles(bundles[:MaxProviderCABundleCerts]...)
				Expect(err).ToNot(HaveOccurred())
				_, err = MergeProviderCABundles(bundles...)
				Expect(err).To(MatchError(ContainSubstring("exceeds")))
			})

			It("rejects a nil bundle", func() {
				_, err := MergeProviderCABundles(parse(ca.pem), nil)
				Expect(err).To(HaveOccurred())
			})
		})

		It("does not reject future-dated certificates at load time", func() {
			t := tmpl("future", true)
			t.NotBefore = time.Now().Add(24 * time.Hour)
			t.NotAfter = time.Now().Add(48 * time.Hour)
			_, err := ParseProviderCABundle(newCert(t, nil).pem)
			Expect(err).ToNot(HaveOccurred())
		})

		It("fails when system roots are unavailable", func() {
			SystemCertPool = func() (*x509.CertPool, error) { return nil, errors.New("boom") }
			b, err := ParseProviderCABundle(ca.pem)
			Expect(err).ToNot(HaveOccurred())
			_, err = b.TLSConfig(nil)
			Expect(err).To(MatchError(ContainSubstring("system roots")))
		})

		It("refuses InsecureSkipVerify", func() {
			b, _ := ParseProviderCABundle(ca.pem)
			_, err := b.TLSConfig(&tls.Config{InsecureSkipVerify: true})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("TLS verification", func() {
		It("trusts a server signed by the CA", func() {
			srv := serve(leaf)
			defer srv.Close()
			b, err := ParseProviderCABundle(ca.pem)
			Expect(err).ToNot(HaveOccurred())
			Expect(get(b, srv.URL)).To(Succeed())
		})

		It("keeps system-only behavior without a bundle", func() {
			srv := serve(leaf)
			defer srv.Close()
			Expect(get(nil, srv.URL)).To(HaveOccurred())
		})

		It("rejects a wrong host name", func() {
			wrong := newCert(tmpl("wrong", false, "example.invalid"), &ca)
			srv := serve(wrong)
			defer srv.Close()
			b, _ := ParseProviderCABundle(ca.pem)
			Expect(get(b, srv.URL)).To(HaveOccurred())
		})

		It("rejects an expired server certificate", func() {
			t := tmpl("expired", false, "127.0.0.1")
			t.NotAfter = time.Now().Add(-time.Minute)
			srv := serve(newCert(t, &ca))
			defer srv.Close()
			b, _ := ParseProviderCABundle(ca.pem)
			Expect(get(b, srv.URL)).To(HaveOccurred())
		})

		It("accepts an exact self-signed non-CA leaf", func() {
			self := newCert(tmpl("self", false, "127.0.0.1"), nil)
			srv := serve(self)
			defer srv.Close()
			b, err := ParseProviderCABundle(self.pem)
			Expect(err).ToNot(HaveOccurred())
			Expect(get(b, srv.URL)).To(Succeed())
		})

		It("rejects a certificate signed by a self-signed non-CA anchor", func() {
			anchor := newCert(tmpl("anchor", false, "127.0.0.1"), nil)
			child := newCert(tmpl("child", false, "127.0.0.1"), &anchor)
			srv := serve(child)
			defer srv.Close()
			b, err := ParseProviderCABundle(anchor.pem)
			Expect(err).ToNot(HaveOccurred())
			// The standard verifier rejects it; VerifyConnection is a second, exact-leaf guard.
			Expect(get(b, srv.URL)).To(HaveOccurred())
		})

		It("restricts non-CA anchors to exact leaf chains after verification", func() {
			anchor := newCert(tmpl("anchor", false, "127.0.0.1"), nil)
			child := newCert(tmpl("child", false, "127.0.0.1"), &anchor)
			b, err := ParseProviderCABundle(append(anchor.pem, ca.pem...))
			Expect(err).ToNot(HaveOccurred())
			cfg, err := b.TLSConfig(nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.InsecureSkipVerify).To(BeFalse())
			Expect(cfg.VerifyConnection(tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{anchor.cert}}})).To(Succeed())
			Expect(cfg.VerifyConnection(tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{leaf.cert, ca.cert}}})).To(Succeed())
			Expect(cfg.VerifyConnection(tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{child.cert, anchor.cert}}})).ToNot(Succeed())
		})

		Context("session resumption", func() {
			var (
				ln    net.Listener
				cache tls.ClientSessionCache
				addr  string
			)

			BeforeEach(func() {
				var err error
				ln, err = tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
					MinVersion:   tls.VersionTLS13,
					Certificates: []tls.Certificate{{Certificate: [][]byte{leaf.cert.Raw}, PrivateKey: leaf.key}},
				})
				Expect(err).ToNot(HaveOccurred())
				addr = ln.Addr().String()
				cache = tls.NewLRUClientSessionCache(4)
				go func() {
					for {
						conn, err := ln.Accept()
						if err != nil {
							return
						}
						go func(c net.Conn) {
							defer c.Close()
							_, _ = c.Write([]byte{1})
							_, _ = c.Read(make([]byte, 1))
						}(conn)
					}
				}()
			})
			AfterEach(func() { Expect(ln.Close()).To(Succeed()) })

			// dial performs a handshake, reads one byte so the session ticket is processed,
			// and reports whether the session was resumed.
			dial := func(cfg *tls.Config) (bool, error) {
				conn, err := tls.Dial("tcp", addr, cfg)
				if err != nil {
					return false, err
				}
				defer conn.Close()
				Expect(conn.SetDeadline(time.Now().Add(5 * time.Second))).To(Succeed())
				if _, err := conn.Read(make([]byte, 1)); err != nil {
					return false, err
				}
				return conn.ConnectionState().DidResume, nil
			}

			// instrument records the DidResume value of each VerifyConnection call.
			instrument := func(cfg *tls.Config, calls *[]bool) *tls.Config {
				inner := cfg.VerifyConnection
				Expect(inner).ToNot(BeNil())
				cfg.ClientSessionCache = cache
				cfg.VerifyConnection = func(cs tls.ConnectionState) error {
					*calls = append(*calls, cs.DidResume)
					return inner(cs)
				}
				return cfg
			}

			It("runs the bundle guard on a resumed session and accepts it", func() {
				b, err := ParseProviderCABundle(ca.pem)
				Expect(err).ToNot(HaveOccurred())
				cfg, err := b.TLSConfig(nil)
				Expect(err).ToNot(HaveOccurred())
				var calls []bool
				cfg = instrument(cfg, &calls)

				resumed, err := dial(cfg)
				Expect(err).ToNot(HaveOccurred())
				Expect(resumed).To(BeFalse())
				resumed, err = dial(cfg)
				Expect(err).ToNot(HaveOccurred())
				Expect(resumed).To(BeTrue())
				Expect(calls).To(Equal([]bool{false, true}))
			})

			It("rejects a resumed session when the bundle guard fails", func() {
				b, err := ParseProviderCABundle(ca.pem)
				Expect(err).ToNot(HaveOccurred())
				cfg, err := b.TLSConfig(nil)
				Expect(err).ToNot(HaveOccurred())
				var first []bool
				_, err = dial(instrument(cfg, &first))
				Expect(err).ToNot(HaveOccurred())

				// Same roots and session cache, but the CA is now treated as an exact-leaf anchor,
				// so the guard rejects the cached [leaf, ca] chain.
				strict := &ProviderCABundle{certs: b.certs, exactLeafs: map[string]struct{}{string(ca.cert.Raw): {}}}
				strictCfg, err := strict.TLSConfig(nil)
				Expect(err).ToNot(HaveOccurred())
				strictCfg.RootCAs = cfg.RootCAs
				var calls []bool
				_, err = dial(instrument(strictCfg, &calls))
				Expect(err).To(MatchError(ContainSubstring("non-CA provider anchor")))
				Expect(calls).To(Equal([]bool{true}))
			})
		})

		It("denies redirects", func() {
			target := serve(leaf)
			defer target.Close()
			srv := httptest.NewUnstartedServer(http.RedirectHandler(target.URL, http.StatusFound))
			srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leaf.cert.Raw}, PrivateKey: leaf.key}}}
			srv.StartTLS()
			defer srv.Close()
			b, _ := ParseProviderCABundle(ca.pem)
			Expect(get(b, srv.URL)).To(MatchError(ContainSubstring("redirects are not allowed")))
		})

		It("builds separate transports with separate pools", func() {
			other := newCert(tmpl("other", true), nil)
			apiB, _ := ParseProviderCABundle(ca.pem)
			tokB, _ := ParseProviderCABundle(other.pem)
			apiT, err := NewProviderTransport(nil, apiB)
			Expect(err).ToNot(HaveOccurred())
			tokT, err := NewProviderTransport(nil, tokB)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiT.TLSClientConfig.RootCAs).ToNot(BeIdenticalTo(tokT.TLSClientConfig.RootCAs))
			srv := serve(leaf)
			defer srv.Close()
			Expect(get(tokB, srv.URL)).To(HaveOccurred())
			Expect(get(apiB, srv.URL)).To(Succeed())
		})
	})
})
