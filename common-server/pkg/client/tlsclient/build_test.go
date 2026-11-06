// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tlsclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/oauth2"
)

var _ = Describe("NewCertPool", func() {
	It("fails when system roots cannot be loaded", func() {
		orig := systemCertPool
		DeferCleanup(func() { systemCertPool = orig })
		systemCertPool = func() (*x509.CertPool, error) { return nil, errors.New("boom") }

		_, err := NewCertPool(nil)
		Expect(err).To(MatchError(ContainSubstring("boom")))
	})

	It("rejects bytes without certificates", func() {
		_, err := NewCertPool([]byte("junk"))
		Expect(err).To(MatchError(ErrInvalidCA))
	})

	It("keeps system roots and adds custom CAs", func() {
		sys := newTestCA("system")
		custom := newTestCA("custom")
		orig := systemCertPool
		DeferCleanup(func() { systemCertPool = orig })
		systemCertPool = func() (*x509.CertPool, error) {
			p := x509.NewCertPool()
			p.AddCert(sys.cert)
			return p, nil
		}
		pool, err := NewCertPool(custom.pem)
		Expect(err).NotTo(HaveOccurred())
		for _, ca := range []*testCA{sys, custom} {
			_, err := ca.cert.Verify(x509.VerifyOptions{Roots: pool})
			Expect(err).NotTo(HaveOccurred())
		}
	})
})

var _ = Describe("Profile and Spec", func() {
	It("defaults the timeout", func() {
		Expect(Profile{}.timeout()).To(Equal(DefaultTimeout))
		Expect(Profile{Timeout: time.Second}.timeout()).To(Equal(time.Second))
	})

	DescribeTable("rejects invalid profiles",
		func(p Profile) { Expect(p.validate()).To(HaveOccurred()) },
		Entry("no name", Profile{}),
		Entry("negative timeout", Profile{Name: "p", Timeout: -1}),
		Entry("unknown transport", Profile{Name: "p", Transport: 7}),
		Entry("invalid pattern", Profile{Name: "p", MetricsReplacePatterns: []string{"("}}),
	)

	DescribeTable("rejects invalid specs",
		func(s Spec) { Expect(s.validate()).To(HaveOccurred()) },
		Entry("relative api url", Spec{APIURL: "/x", Auth: Auth{Mode: AuthNone}}),
		Entry("missing token url", Spec{APIURL: "https://a", Auth: Auth{Mode: AuthPassword}}),
		Entry("unknown mode", Spec{APIURL: "https://a", Auth: Auth{Mode: "x"}}),
		Entry("empty mode", Spec{APIURL: "https://a"}),
	)

	It("fingerprints every relevant field", func() {
		base := Spec{APIURL: "https://a", Auth: Auth{Mode: AuthPassword, TokenURL: "https://t", ClientID: "c", ClientSecret: "s", Username: "u", Password: "p"}}
		fp := fingerprint("p", base, []byte("ca"))
		Expect(fingerprint("p", base, []byte("ca"))).To(Equal(fp))
		variants := []func(*Spec){
			func(s *Spec) { s.APIURL = "https://b" },
			func(s *Spec) { s.Auth.Mode = AuthClientCredentials },
			func(s *Spec) { s.Auth.TokenURL = "https://t2" },
			func(s *Spec) { s.Auth.ClientID = "c2" },
			func(s *Spec) { s.Auth.ClientSecret = "s2" },
			func(s *Spec) { s.Auth.Username = "u2" },
			func(s *Spec) { s.Auth.Password = "p2" },
		}
		for _, v := range variants {
			s := base
			v(&s)
			Expect(fingerprint("p", s, []byte("ca"))).NotTo(Equal(fp))
		}
		Expect(fingerprint("q", base, []byte("ca"))).NotTo(Equal(fp))
		Expect(fingerprint("p", base, []byte("cb"))).NotTo(Equal(fp))
		// field boundaries are unambiguous
		a := Spec{APIURL: "https://ab", Auth: Auth{Mode: AuthNone}}
		Expect(fingerprint("p", a, nil)).NotTo(Equal(fingerprint("p", a, []byte("x"))))
	})

	It("distinguishes values shifted across adjacent field boundaries", func() {
		ab := Spec{APIURL: "https://a", Auth: Auth{Mode: AuthClientCredentials, TokenURL: "https://t", ClientID: "ab", ClientSecret: "c"}}
		a := ab
		a.Auth.ClientID, a.Auth.ClientSecret = "a", "bc"
		Expect(fingerprint("p", ab, nil)).NotTo(Equal(fingerprint("p", a, nil)))
	})
})

var _ = Describe("New", func() {
	var ctx context.Context
	profile := Profile{Name: "test", Timeout: 2 * time.Second, MetricsClientName: "tlsclient-test", MetricsReplacePatterns: []string{"[0-9]+"}}

	BeforeEach(func() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
	})

	It("verifies the server with the custom CA", func() {
		ca := newTestCA("ca")
		srv := startTLSServer(ca.serverCert(""), &apiHandler{})

		c, err := New(ctx, profile, Spec{APIURL: srv.URL, Auth: Auth{Mode: AuthNone}, CA: &staticCA{pem: ca.pem}})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(c.Close)
		Expect(c.APIURL()).To(Equal(srv.URL))
		Expect(get(c, srv.URL+"/x/123")).To(Equal(http.StatusOK))
	})

	It("rejects servers without trusted CA", func() {
		srv := startTLSServer(newTestCA("ca").serverCert(""), &apiHandler{})
		c, err := New(ctx, profile, Spec{APIURL: srv.URL, Auth: Auth{Mode: AuthNone}, CA: &staticCA{pem: newTestCA("other").pem}})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(c.Close)
		_, err = get(c, srv.URL)
		var uaErr x509.UnknownAuthorityError
		Expect(errors.As(err, &uaErr)).To(BeTrue(), fmt.Sprint(err))
	})

	It("verifies the hostname", func() {
		ca := newTestCA("ca")
		srv := startTLSServer(ca.serverCert("other.example"), &apiHandler{})
		c, err := New(ctx, profile, Spec{APIURL: srv.URL, Auth: Auth{Mode: AuthNone}, CA: &staticCA{pem: ca.pem}})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(c.Close)
		_, err = get(c, srv.URL)
		var hErr x509.HostnameError
		Expect(errors.As(err, &hErr)).To(BeTrue(), fmt.Sprint(err))
	})

	It("skips verification only when explicitly requested", func() {
		srv := startTLSServer(newTestCA("ca").serverCert("other.example"), &apiHandler{})
		c, err := New(ctx, profile, Spec{APIURL: srv.URL, Auth: Auth{Mode: AuthNone}}, WithInsecureSkipVerify(true))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(c.Close)
		Expect(get(c, srv.URL)).To(Equal(http.StatusOK))
	})

	It("propagates resolver errors", func() {
		_, err := New(ctx, profile, Spec{APIURL: "https://a", Auth: Auth{Mode: AuthNone}, CA: &staticCA{err: errors.New("bad ca")}})
		Expect(err).To(MatchError(ContainSubstring("bad ca")))
	})

	It("rejects invalid profiles and specs", func() {
		_, err := New(ctx, Profile{}, Spec{APIURL: "https://a", Auth: Auth{Mode: AuthNone}})
		Expect(err).To(HaveOccurred())
		_, err = New(ctx, profile, Spec{})
		Expect(err).To(HaveOccurred())
	})

	It("applies transport settings", func() {
		t, err := newTransport(Profile{Name: "p", MaxIdleConnsPerHost: 100, MinTLSVersion: tls.VersionTLS13}, nil, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.MaxIdleConnsPerHost).To(Equal(100))
		Expect(t.TLSClientConfig.MinVersion).To(Equal(uint16(tls.VersionTLS13)))
		Expect(t.TLSClientConfig.InsecureSkipVerify).To(BeFalse())
		Expect(t.Proxy).To(BeNil())

		t, err = newTransport(Profile{Name: "p", Transport: TransportDefault}, nil, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.Proxy).NotTo(BeNil())
		Expect(t.TLSClientConfig.RootCAs).NotTo(BeNil())
	})

	Context("client credentials", func() {
		It("uses the custom CAs for token and API endpoints", func() {
			tokenCA, apiCA := newTestCA("token"), newTestCA("api")
			ts := &tokenServer{}
			tokenSrv := startTLSServer(tokenCA.serverCert(""), ts)
			api := &apiHandler{}
			apiSrv := startTLSServer(apiCA.serverCert(""), api)
			spec := Spec{APIURL: apiSrv.URL, Auth: Auth{Mode: AuthClientCredentials, TokenURL: tokenSrv.URL, ClientID: "id", ClientSecret: "secret"}}

			spec.CA = &staticCA{pem: join(tokenCA.pem, apiCA.pem)}
			c, err := New(ctx, profile, spec)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(c.Close)
			Expect(get(c, apiSrv.URL)).To(Equal(http.StatusOK))
			Expect(api.last()).To(Equal("Bearer token-client_credentials"))

			spec.CA = &staticCA{pem: apiCA.pem}
			c2, err := New(ctx, profile, spec)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(c2.Close)
			_, err = get(c2, apiSrv.URL)
			var uaErr x509.UnknownAuthorityError
			Expect(errors.As(err, &uaErr)).To(BeTrue(), fmt.Sprint(err))
		})

		It("bounds token requests with the default timeout when no timeout is set", func() {
			ca := newTestCA("ca")
			tokenSrv := startTLSServer(ca.serverCert(""), &tokenServer{})
			c, err := New(ctx, Profile{Name: "p"}, Spec{APIURL: tokenSrv.URL, Auth: Auth{Mode: AuthClientCredentials, TokenURL: tokenSrv.URL}, CA: &staticCA{pem: ca.pem}})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(c.Close)
			Expect(c.Doer().(*http.Client).Timeout).To(Equal(DefaultTimeout))
		})
	})

	Context("password", func() {
		var (
			ca       *testCA
			ts       *tokenServer
			api      *apiHandler
			spec     Spec
			tokenURL string
		)

		BeforeEach(func() {
			ca = newTestCA("ca")
			ts = &tokenServer{}
			tokenURL = startTLSServer(ca.serverCert(""), ts).URL
			api = &apiHandler{}
			apiURL := startTLSServer(ca.serverCert(""), api).URL
			spec = Spec{APIURL: apiURL, Auth: Auth{Mode: AuthPassword, TokenURL: tokenURL, ClientID: "admin-cli", Username: "u", Password: "p"}, CA: &staticCA{pem: ca.pem}}
		})

		It("authenticates eagerly", func() {
			c, err := New(ctx, profile, spec)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(c.Close)
			Expect(ts.passwordCalls.Load()).To(Equal(int32(1)))
			Expect(get(c, spec.APIURL)).To(Equal(http.StatusOK))
			Expect(api.last()).To(Equal("Bearer token-password"))
		})

		It("fails when initial authentication fails", func() {
			ts.passwordStatus.Store(http.StatusUnauthorized)
			_, err := New(ctx, profile, spec)
			var re *oauth2.RetrieveError
			Expect(errors.As(err, &re)).To(BeTrue())
		})

		It("fails initial authentication within the timeout", func() {
			blocked := &tokenServer{block: make(chan struct{})}
			spec.Auth.TokenURL = startTLSServer(ca.serverCert(""), blocked).URL
			DeferCleanup(func() { close(blocked.block) })
			start := time.Now()
			_, err := New(ctx, Profile{Name: "p", Timeout: 200 * time.Millisecond}, spec)
			Expect(err).To(HaveOccurred())
			Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
		})

		It("re-authenticates when the refresh token is rejected", func() {
			ts.expiresIn.Store(1) // expired immediately due to oauth2 expiry delta
			c, err := New(ctx, profile, spec)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(c.Close)

			ts.refreshStatus.Store(http.StatusUnauthorized)
			Expect(get(c, spec.APIURL)).To(Equal(http.StatusOK))
			Expect(ts.refreshCalls.Load()).To(BeNumerically(">=", 1))
			Expect(ts.passwordCalls.Load()).To(BeNumerically(">=", 2))
		})

		It("classifies transient refresh errors without re-authentication", func() {
			ts.expiresIn.Store(1)
			var classified []int
			p := profile
			p.ClassifyTokenError = func(err error) error {
				var re *oauth2.RetrieveError
				if errors.As(err, &re) {
					classified = append(classified, re.Response.StatusCode)
				}
				return fmt.Errorf("classified: %w", err)
			}
			c, err := New(ctx, p, spec)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(c.Close)

			ts.refreshStatus.Store(http.StatusServiceUnavailable)
			_, err = get(c, spec.APIURL)
			Expect(err).To(MatchError(ContainSubstring("classified")))
			Expect(classified).To(Equal([]int{http.StatusServiceUnavailable}))
			Expect(ts.passwordCalls.Load()).To(Equal(int32(1)))
		})

		It("classifies failed re-authentication", func() {
			ts.expiresIn.Store(1)
			c, err := New(ctx, profile, spec)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(c.Close)

			ts.refreshStatus.Store(http.StatusBadRequest)
			ts.passwordStatus.Store(http.StatusTooManyRequests)
			_, err = get(c, spec.APIURL)
			Expect(err).To(MatchError(ContainSubstring("re-authentication failed")))
		})

		It("stops refreshing after Close", func() {
			ts.expiresIn.Store(1)
			c, err := New(ctx, profile, spec)
			Expect(err).NotTo(HaveOccurred())
			c.Close()
			c.Close()
			_, err = get(c, spec.APIURL)
			Expect(err).To(MatchError(context.Canceled))
		})
	})
})
