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

	It("rejects issuer mismatch, non-2xx responses, and insecure token endpoints", func() {
		var issuer string
		status := http.StatusOK
		body := func() string {
			return fmt.Sprintf(`{"issuer":%q,"token_endpoint":"https://tokens.example.com/token"}`, issuer)
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status); _, _ = w.Write([]byte(body())) }))
		defer server.Close()
		issuer = server.URL

		body = func() string {
			return `{"issuer":"https://wrong.example.com","token_endpoint":"https://tokens.example.com/token"}`
		}
		_, err := discoverTokenURL(context.Background(), server.Client(), issuer)
		Expect(err).To(HaveOccurred())

		status = http.StatusBadGateway
		_, err = discoverTokenURL(context.Background(), server.Client(), issuer)
		Expect(err).To(HaveOccurred())

		status = http.StatusOK
		body = func() string {
			return fmt.Sprintf(`{"issuer":%q,"token_endpoint":"http://tokens.example.com/token"}`, issuer)
		}
		_, err = discoverTokenURL(context.Background(), server.Client(), issuer)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Preset OIDC status", func() {
	newContext := func(client *http.Client) *HandlingContext {
		zone := &adminv1.Zone{
			Spec: adminv1.ZoneSpec{
				IdentityProviders: []adminv1.IdentityProviderConfig{{Name: "primary", IssuerHostname: "issuer.example.com"}},
				Gateways:          []adminv1.GatewayConfig{{Name: "standard"}},
				Presets: []adminv1.Preset{
					{Name: "default", Default: true, GatewayRef: "standard", IdentityProviderRef: "primary", Urls: []adminv1.UrlConfig{{Hostname: "api.example.com", BasePath: "/"}}},
					{Name: "failover", GatewayRef: "standard", IdentityProviderRef: "primary", Urls: []adminv1.UrlConfig{{Hostname: "failover.example.com", BasePath: "/"}}},
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
		Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("TokenUrl", "https://tokens.example.com/token")))
	})

	It("reuses a safe status token URL only for the matching issuer", func() {
		hc := newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "default", Links: adminv1.Links{Issuer: "https://issuer.example.com/auth/realms/default"}, TokenUrl: "https://tokens.example.com/token",
		}}

		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("TokenUrl", "https://tokens.example.com/token")))

		hc = newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "default", Links: adminv1.Links{Issuer: "https://other.example.com/auth/realms/"}, TokenUrl: "https://tokens.example.com/token",
		}}
		Expect(populatePresetStatus(context.Background(), hc)).To(HaveOccurred())

		hc = newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "removed", Links: adminv1.Links{Issuer: "https://issuer.example.com/auth/realms/default"}, TokenUrl: "https://tokens.example.com/token",
		}}
		Expect(populatePresetStatus(context.Background(), hc)).To(HaveOccurred())
	})

	It("does not reuse an invalid status token URL", func() {
		hc := newContext(&http.Client{Transport: failingRoundTripper{}})
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{
			Name: "default", Links: adminv1.Links{Issuer: "https://issuer.example.com/auth/realms/default"}, TokenUrl: "https://tokens.example.com/token?secret=value",
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
