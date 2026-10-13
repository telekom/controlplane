// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network unavailable")
}

var _ = Describe("OIDC discovery", func() {
	It("returns a contextual error for a nil HTTP client", func() {
		_, err := discoverTokenURL(context.Background(), nil, "https://issuer.example.com")
		Expect(err).To(MatchError(ContainSubstring("OIDC discovery HTTP client")))
	})

	It("returns a validated HTTPS token endpoint", func() {
		var issuer string
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/issuer/.well-known/openid-configuration"))
			_, err := fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"https://tokens.example.com/token"}`, issuer)
			Expect(err).NotTo(HaveOccurred())
		}))
		defer server.Close()
		issuer = server.URL + "/issuer"
		tokenURL, err := discoverTokenURL(context.Background(), server.Client(), issuer)
		Expect(err).NotTo(HaveOccurred())
		Expect(tokenURL).To(Equal("https://tokens.example.com/token"))
	})

	It("accepts an equivalent issuer with a trailing slash", func() {
		var issuer string
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"https://tokens.example.com/token"}`, issuer+"/")
			Expect(err).NotTo(HaveOccurred())
		}))
		defer server.Close()
		issuer = server.URL

		_, err := discoverTokenURL(context.Background(), server.Client(), issuer)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects malformed and oversized metadata", func() {
		var body string
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, body)
		}))
		defer server.Close()

		body = `{not-json}`
		_, err := discoverTokenURL(context.Background(), server.Client(), server.URL)
		Expect(err).To(MatchError(ContainSubstring("decoding OIDC metadata")))

		body = fmt.Sprintf(`{"issuer":%q,"token_endpoint":"https://tokens.example.com/token","padding":%q}`, server.URL, strings.Repeat("x", 1<<20))
		_, err = discoverTokenURL(context.Background(), server.Client(), server.URL)
		Expect(err).To(MatchError(ContainSubstring("exceeds")))
	})

	It("rejects token endpoints with query parameters", func() {
		var server *httptest.Server
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"https://tokens.example.com/token?audience=api"}`, server.URL)
			Expect(err).NotTo(HaveOccurred())
		}))
		defer server.Close()

		_, err := discoverTokenURL(context.Background(), server.Client(), server.URL)
		Expect(err).To(MatchError(ContainSubstring("query")))
	})

	DescribeTable("does not compare the advertised issuer with the configured issuer",
		func(metadata string) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(metadata)) }))
			defer server.Close()

			tokenURL, err := discoverTokenURL(context.Background(), server.Client(), server.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenURL).To(Equal("https://tokens.example.com/token"))
		},
		Entry("mismatched issuer", `{"issuer":"https://wrong.example.com","token_endpoint":"https://tokens.example.com/token"}`),
		Entry("empty issuer", `{"issuer":"","token_endpoint":"https://tokens.example.com/token"}`),
		Entry("absent issuer", `{"token_endpoint":"https://tokens.example.com/token"}`),
	)

	It("rejects non-2xx responses", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprintf(w, `{"token_endpoint":"https://tokens.example.com/token"}`)
		}))
		defer server.Close()

		_, err := discoverTokenURL(context.Background(), server.Client(), server.URL)
		Expect(err).To(MatchError(ContainSubstring("HTTP 502")))
	})

	DescribeTable("rejects unsafe token endpoints regardless of the advertised issuer",
		func(issuer string) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if issuer == "" {
					issuer = server.URL
				}
				_, _ = fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"http://tokens.example.com/token"}`, issuer)
			}))
			defer server.Close()

			_, err := discoverTokenURL(context.Background(), server.Client(), server.URL)
			Expect(err).To(MatchError(ContainSubstring("absolute HTTPS URL")))
		},
		Entry("matching issuer", ""),
		Entry("mismatched issuer", "https://wrong.example.com"),
	)
})

var _ = Describe("OIDC discovery TLS verification", func() {
	var server *httptest.Server

	BeforeEach(func() {
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"https://tokens.example.com/token"}`, server.URL)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)

		previous, wasSet := os.LookupEnv(EnvSkipTlsVerify)
		DeferCleanup(func() {
			if wasSet {
				Expect(os.Setenv(EnvSkipTlsVerify, previous)).To(Succeed())
				return
			}
			Expect(os.Unsetenv(EnvSkipTlsVerify)).To(Succeed())
		})
	})

	It("rejects an untrusted certificate when SKIP_TLS_VERIFY is unset", func() {
		Expect(os.Unsetenv(EnvSkipTlsVerify)).To(Succeed())
		client := (&ZoneHandler{}).httpClient()
		Expect(client.Transport).To(BeNil())

		_, err := discoverTokenURL(context.Background(), client, server.URL)
		Expect(err).To(MatchError(ContainSubstring("certificate")))
	})

	DescribeTable("keeps verification for values other than true",
		func(value string) {
			Expect(os.Setenv(EnvSkipTlsVerify, value)).To(Succeed())
			client := (&ZoneHandler{}).httpClient()
			Expect(client.Transport).To(BeNil())

			_, err := discoverTokenURL(context.Background(), client, server.URL)
			Expect(err).To(MatchError(ContainSubstring("certificate")))
		},
		Entry("false", "false"),
		Entry("empty", ""),
		Entry("invalid", "yes"),
		Entry("numeric", "1"),
	)

	DescribeTable("accepts an untrusted certificate when SKIP_TLS_VERIFY is true",
		func(value string) {
			Expect(os.Setenv(EnvSkipTlsVerify, value)).To(Succeed())

			tokenURL, err := discoverTokenURL(context.Background(), (&ZoneHandler{}).httpClient(), server.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenURL).To(Equal("https://tokens.example.com/token"))
		},
		Entry("lowercase", "true"),
		Entry("mixed case with whitespace", " TRUE "),
	)

	It("does not modify the default transport", func() {
		Expect(os.Setenv(EnvSkipTlsVerify, "true")).To(Succeed())
		_ = (&ZoneHandler{}).httpClient()

		defaultTransport := http.DefaultTransport.(*http.Transport)
		Expect(defaultTransport.TLSClientConfig == nil || !defaultTransport.TLSClientConfig.InsecureSkipVerify).To(BeTrue())
	})

	DescribeTable("NewZoneHandler builds the discovery client once from SKIP_TLS_VERIFY",
		func(value string, accepts bool) {
			Expect(os.Setenv(EnvSkipTlsVerify, value)).To(Succeed())
			h := NewZoneHandler()
			Expect(h.HTTPClient).NotTo(BeNil())
			Expect(h.httpClient()).To(BeIdenticalTo(h.HTTPClient))

			_, err := discoverTokenURL(context.Background(), h.httpClient(), server.URL)
			if accepts {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(MatchError(ContainSubstring("certificate")))
		},
		Entry("true", "true", true),
		Entry("false", "false", false),
	)

	It("keeps an injected HTTP client", func() {
		Expect(os.Setenv(EnvSkipTlsVerify, "true")).To(Succeed())
		injected := &http.Client{}
		Expect((&ZoneHandler{HTTPClient: injected}).httpClient()).To(BeIdenticalTo(injected))
	})
})

var _ = Describe("Preset OIDC status", func() {
	newContext := func(client *http.Client) *HandlingContext {
		zone := &adminv1.Zone{
			Spec: adminv1.ZoneSpec{
				IdentityProviders: []adminv1.IdentityProviderConfig{{Name: "primary", IssuerHostname: "issuer.example.com"}},
				Gateways:          []adminv1.GatewayConfig{{Name: "standard"}},
				Presets: []adminv1.Preset{
					{Name: "default", Type: adminv1.GatewayTypeAPI, Default: true, GatewayRef: "standard", IdentityProviderRef: "primary", Urls: []adminv1.UrlConfig{{Hostname: "api.example.com", BasePath: "/"}}},
					{Name: "failover", Type: adminv1.GatewayTypeAPI, GatewayRef: "standard", IdentityProviderRef: "primary", Urls: []adminv1.UrlConfig{{Hostname: "failover.example.com", BasePath: "/"}}, Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
				},
			},
			Status: adminv1.ZoneStatus{
				IdentityProvider: &commontypes.ObjectRef{Name: "identity"},
				Gateways:         []adminv1.GatewayStatus{{Name: "standard", Gateway: &commontypes.ObjectRef{Name: "gateway"}}},
			},
		}
		return &HandlingContext{
			Zone: zone, HTTPClient: client,
			DefaultIdentityRealm:  &identityapi.Realm{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			InternalIdentityRealm: &identityapi.Realm{ObjectMeta: metav1.ObjectMeta{Name: "internal"}},
		}
	}

	It("bubbles network failures as retryable", func() {
		hc := newContext(&http.Client{Transport: failingRoundTripper{}})
		err := populatePresetStatus(context.Background(), hc)
		var retryable ctrlerrors.RetryableError
		Expect(errors.As(err, &retryable)).To(BeTrue())
		Expect(retryable.IsRetryable()).To(BeTrue())
	})

	It("discovers once for presets with the same issuer", func() {
		requests := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests++
			_, err := fmt.Fprint(w, `{"issuer":"https://issuer.example.com/auth/realms/default","token_endpoint":"https://tokens.example.com/token"}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		defer server.Close()
		hc := newContext(server.Client())
		hc.HTTPClient.Transport = rewriteHostTransport{base: hc.HTTPClient.Transport, target: server.URL}

		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(requests).To(Equal(1))
		Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("Links.TokenUrl", "https://tokens.example.com/token")))
	})

	It("accepts an advertised issuer that differs from the configured issuer", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := fmt.Fprint(w, `{"issuer":"https://kong.example.com/auth/realms/default","token_endpoint":"https://kong.example.com/auth/realms/default/protocol/openid-connect/token"}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		defer server.Close()
		hc := newContext(server.Client())
		hc.Zone.Spec.IdentityProviders[0].IssuerHostname = "keycloak.intra.example.com"
		hc.HTTPClient.Transport = rewriteHostTransport{base: hc.HTTPClient.Transport, target: server.URL}

		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		preset := hc.Zone.Status.Presets[0]
		Expect(preset.Name).To(Equal("default"))
		Expect(preset.Links.Issuer).To(Equal("https://keycloak.intra.example.com/auth/realms/default"))
		Expect(preset.Links.TokenUrl).To(Equal("https://kong.example.com/auth/realms/default/protocol/openid-connect/token"))
	})

	It("reuses a safe status token URL only for the matching issuer", func() {
		hc := newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "default", Links: adminv1.Links{Issuer: "https://issuer.example.com/auth/realms/default", TokenUrl: "https://tokens.example.com/token"},
		}}

		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("Links.TokenUrl", "https://tokens.example.com/token")))

		hc = newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "default", Links: adminv1.Links{Issuer: "https://other.example.com/auth/realms/", TokenUrl: "https://tokens.example.com/token"},
		}}
		Expect(populatePresetStatus(context.Background(), hc)).To(HaveOccurred())

		hc = newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "removed", Links: adminv1.Links{Issuer: "https://issuer.example.com/auth/realms/default", TokenUrl: "https://tokens.example.com/token"},
		}}
		Expect(populatePresetStatus(context.Background(), hc)).To(HaveOccurred())
	})

	It("does not reuse an invalid status token URL", func() {
		hc := newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "default", Links: adminv1.Links{Issuer: "https://issuer.example.com/auth/realms/default", TokenUrl: "https://tokens.example.com/token?secret=value"},
		}}

		Expect(populatePresetStatus(context.Background(), hc)).To(HaveOccurred())
	})
})

type rewriteHostTransport struct {
	base   http.RoundTripper
	target string
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	targetReq := req.Clone(req.Context())
	var err error
	targetReq.URL, err = targetReq.URL.Parse(t.target + req.URL.Path)
	if err != nil {
		return nil, err
	}
	return t.base.RoundTrip(targetReq)
}
