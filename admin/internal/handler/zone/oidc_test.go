// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const defaultRealmIssuer = "https://kong.example.com/auth/realms/default"

func expectRetryableWith(err error, substring string) {
	GinkgoHelper()
	var retryable ctrlerrors.RetryableError
	Expect(errors.As(err, &retryable)).To(BeTrue())
	Expect(retryable.IsRetryable()).To(BeTrue())
	Expect(err).To(MatchError(ContainSubstring(substring)))
}

var _ = Describe("Preset OIDC status", func() {
	newContext := func() *HandlingContext {
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
			Zone: zone,
			DefaultIdentityRealm: &identityapi.Realm{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
				Status:     identityapi.RealmStatus{IssuerUrl: defaultRealmIssuer},
			},
			InternalIdentityRealm: &identityapi.Realm{
				ObjectMeta: metav1.ObjectMeta{Name: "internal"},
				Status:     identityapi.RealmStatus{IssuerUrl: "https://irix.example.com/auth/realms/internal"},
			},
		}
	}

	It("uses the resolved internal realm issuer and keeps the public issuer", func() {
		hc := newContext()
		hc.Zone.Spec.Visibility = adminv1.ZoneVisibilityWorld
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveEach(And(
			HaveField("Links.Issuer", "https://issuer.example.com/auth/realms/default"),
			HaveField("Links.InternalIssuer", "https://irix.example.com/auth/realms/internal"),
		)))
	})

	DescribeTable("trims a trailing slash from the internal issuer without mutating the realm status",
		func(rawIssuer string) {
			hc := newContext()
			hc.InternalIdentityRealm.Status.IssuerUrl = rawIssuer

			issuer, err := internalIssuerURL(hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(issuer).To(Equal("https://irix.example.com/auth/realms/internal"))
			Expect(hc.InternalIdentityRealm.Status.IssuerUrl).To(Equal(rawIssuer))
		},
		Entry("with trailing slash", "https://irix.example.com/auth/realms/internal/"),
		Entry("without trailing slash", "https://irix.example.com/auth/realms/internal"),
	)

	It("returns a retryable error while the internal realm issuer is unresolved", func() {
		hc := newContext()
		hc.InternalIdentityRealm.Status.IssuerUrl = ""
		expectRetryableWith(populatePresetStatus(context.Background(), hc), "no resolved issuer URL")
		Expect(hc.Zone.Status.Presets).To(BeEmpty())
	})

	It("derives the token URL from the default realm status issuer, not the configured hostname", func() {
		hc := newContext()
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveLen(2))
		Expect(hc.Zone.Status.Presets).To(HaveEach(And(
			HaveField("Links.Issuer", "https://issuer.example.com/auth/realms/default"),
			HaveField("Links.TokenUrl", defaultRealmIssuer+"/protocol/openid-connect/token"),
		)))
	})

	It("sorts preset statuses by name", func() {
		hc := newContext()
		hc.Zone.Spec.Presets[0], hc.Zone.Spec.Presets[1] = hc.Zone.Spec.Presets[1], hc.Zone.Spec.Presets[0]
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveExactElements(HaveField("Name", "default"), HaveField("Name", "failover")))
	})

	It("prefers the preset token URL over the identity provider token URL and the realm fallback", func() {
		hc := newContext()
		hc.Zone.Spec.IdentityProviders[0].TokenUrl = "https://idp.example.com/token"
		hc.Zone.Spec.Presets[1].TokenUrl = "https://preset.example.com/token"
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveExactElements(
			HaveField("Links.TokenUrl", "https://idp.example.com/token"),
			HaveField("Links.TokenUrl", "https://preset.example.com/token"),
		))
	})

	It("does not require a resolved realm issuer when token URLs are explicit", func() {
		hc := newContext()
		hc.DefaultIdentityRealm.Status.IssuerUrl = ""
		hc.Zone.Spec.IdentityProviders[0].TokenUrl = "https://idp.example.com/token"
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("Links.TokenUrl", "https://idp.example.com/token")))
	})

	DescribeTable("normalizes the realm issuer",
		func(issuer, expected string) {
			hc := newContext()
			hc.DefaultIdentityRealm.Status.IssuerUrl = issuer
			Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
			Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("Links.TokenUrl", expected)))
			Expect(hc.DefaultIdentityRealm.Status.IssuerUrl).To(Equal(issuer))
		},
		Entry("trailing slashes", "https://kong.example.com/auth/realms/default//", "https://kong.example.com/auth/realms/default/protocol/openid-connect/token"),
		Entry("port and context path", "https://kong.example.com:8443/ctx/auth/realms/default", "https://kong.example.com:8443/ctx/auth/realms/default/protocol/openid-connect/token"),
	)

	It("returns a retryable error when the default realm issuer is unresolved", func() {
		hc := newContext()
		hc.DefaultIdentityRealm.Status.IssuerUrl = ""
		expectRetryableWith(populatePresetStatus(context.Background(), hc), `default identity realm "default" has no resolved issuer URL`)
		Expect(hc.Zone.Status.Presets).To(BeEmpty())
	})

	It("returns a retryable error when the default realm is not resolved", func() {
		hc := newContext()
		hc.DefaultIdentityRealm = nil
		_, err := defaultRealmTokenURL(hc, "default")
		expectRetryableWith(err, "default identity realm is not resolved")

		hc.Zone.Spec.IdentityProviders[0].TokenUrl = "https://idp.example.com/token"
		hc.Zone.Status.Presets = []adminv1.PresetStatus{{Name: "default"}}
		expectRetryableWith(populatePresetStatus(context.Background(), hc), "default identity realm is not resolved")
		Expect(hc.Zone.Status.Presets).To(Equal([]adminv1.PresetStatus{{Name: "default"}}))
	})

	DescribeTable("rejects unsafe derived token URLs",
		func(issuer string) {
			hc := newContext()
			hc.DefaultIdentityRealm.Status.IssuerUrl = issuer
			previous := []adminv1.PresetStatus{{Name: "default", Links: adminv1.Links{TokenUrl: "https://old.example.com/token"}}}
			hc.Zone.Status.Presets = previous
			expectRetryableWith(populatePresetStatus(context.Background(), hc), "token endpoint must")
			Expect(hc.Zone.Status.Presets).To(Equal(previous))
		},
		Entry("plain HTTP", "http://kong.example.com/auth/realms/default"),
		Entry("relative URL", "/auth/realms/default"),
		Entry("userinfo", "https://user:pass@kong.example.com/auth/realms/default"),
		Entry("query", "https://kong.example.com/auth/realms/default?x=1"),
		Entry("bare query", "https://kong.example.com/auth/realms/default?"),
		Entry("fragment", "https://kong.example.com/auth/realms/default#frag"),
		Entry("missing host", "https:///auth/realms/default"),
	)

	It("ignores cached status token URLs", func() {
		hc := newContext()
		hc.Zone.Status.Presets = []adminv1.PresetStatus{
			{Name: "default", Links: adminv1.Links{Issuer: "https://issuer.example.com/auth/realms/default", TokenUrl: "https://stale.example.com/token"}},
			{Name: "removed", Links: adminv1.Links{TokenUrl: "https://stale.example.com/token"}},
		}
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveExactElements(HaveField("Name", "default"), HaveField("Name", "failover")))
		Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("Links.TokenUrl", defaultRealmIssuer+"/protocol/openid-connect/token")))
	})

	It("falls back to the realm issuer once an override is removed and follows issuer changes", func() {
		hc := newContext()
		hc.Zone.Spec.Presets[0].TokenUrl = "https://preset.example.com/token"
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets[0].Links.TokenUrl).To(Equal("https://preset.example.com/token"))

		hc.Zone.Spec.Presets[0].TokenUrl = ""
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets[0].Links.TokenUrl).To(Equal(defaultRealmIssuer + "/protocol/openid-connect/token"))

		hc.DefaultIdentityRealm.Status.IssuerUrl = "https://new.example.com/auth/realms/default"
		Expect(populatePresetStatus(context.Background(), hc)).To(Succeed())
		Expect(hc.Zone.Status.Presets).To(HaveEach(HaveField("Links.TokenUrl", "https://new.example.com/auth/realms/default/protocol/openid-connect/token")))
	})
})
