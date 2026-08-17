// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func identityProvider(name, password string) adminv1.IdentityProviderConfig {
	adminURL := "https://idp.example.com/admin"
	return adminv1.IdentityProviderConfig{
		Name: name,
		Admin: adminv1.IdentityProviderAdminConfig{
			Url:      &adminURL,
			ClientId: "admin-client",
			UserName: "admin",
			Password: password,
		},
		IssuerHostname: "login.example.com",
		TokenUrl:       "https://login.example.com/token",
	}
}

func defaultPreset(name string) adminv1.Preset {
	return adminv1.Preset{
		Name:                name,
		Default:             true,
		GatewayRef:          "standard",
		IdentityProviderRef: "primary",
		TokenUrl:            "https://login.example.com/token",
		Urls: []adminv1.UrlConfig{{
			Hostname: "api.example.com",
			BasePath: "/api/v1",
		}},
	}
}

func failoverPreset(name string) adminv1.Preset {
	preset := defaultPreset(name)
	preset.Default = false
	preset.Features = []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}
	return preset
}

func validZone() *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: "test-zone", Namespace: "default"},
		Spec: adminv1.ZoneSpec{
			Gateways: []adminv1.GatewayConfig{{
				Name: "standard",
				Admin: adminv1.GatewayAdminConfig{
					IdentityProviderRef: "primary",
					Url:                 "https://gateway.example.com/admin",
				},
			}},
			IdentityProviders: []adminv1.IdentityProviderConfig{identityProvider("primary", "secret")},
			Presets:           []adminv1.Preset{defaultPreset("default"), failoverPreset("failover")},
		},
	}
}

var _ = Describe("Zone validation", func() {
	var (
		ctx       context.Context
		validator ZoneCustomValidator
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	DescribeTable("rejects invalid preset graphs on create and update",
		func(mutate func(*adminv1.Zone), message string) {
			zone := validZone()
			mutate(zone)
			_, err := validator.ValidateCreate(ctx, zone)
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(apierrors.IsInvalid(err)).To(BeTrue())

			oldZone := validZone()
			_, err = validator.ValidateUpdate(ctx, oldZone, zone)
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		},
		Entry("no default", func(z *adminv1.Zone) { z.Spec.Presets[0].Default = false }, "exactly one default preset"),
		Entry("two defaults", func(z *adminv1.Zone) { z.Spec.Presets = append(z.Spec.Presets, defaultPreset("other")) }, "exactly one default preset"),
		Entry("missing gateway", func(z *adminv1.Zone) { z.Spec.Presets[0].GatewayRef = "missing" }, "gatewayRef"),
		Entry("missing runtime IDP", func(z *adminv1.Zone) { z.Spec.Presets[0].IdentityProviderRef = "missing" }, "identityProviderRef"),
		Entry("missing admin IDP", func(z *adminv1.Zone) { z.Spec.Gateways[0].Admin.IdentityProviderRef = "missing" }, "identityProviderRef"),
		Entry("multiple IDPs", func(z *adminv1.Zone) {
			z.Spec.IdentityProviders = append(z.Spec.IdentityProviders, identityProvider("secondary", "secret"))
		}, "exactly one identity provider"),
		Entry("ambiguous feature", func(z *adminv1.Zone) { z.Spec.Presets = append(z.Spec.Presets, failoverPreset("other")) }, "multiple presets"),
	)

	It("allows adding and removing non-default presets", func() {
		oldZone := validZone()
		newZone := validZone()
		newZone.Spec.Presets = newZone.Spec.Presets[:1]
		_, err := validator.ValidateUpdate(ctx, oldZone, newZone)
		Expect(err).NotTo(HaveOccurred())

		oldZone = newZone.DeepCopy()
		newZone.Spec.Presets = append(newZone.Spec.Presets, failoverPreset("new-failover"))
		_, err = validator.ValidateUpdate(ctx, oldZone, newZone)
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("requires references to select the sole identity provider",
		func(mutate func(*adminv1.Zone), path string) {
			zone := validZone()
			zone.Spec.IdentityProviders = append(zone.Spec.IdentityProviders, identityProvider("secondary", "secret"))
			mutate(zone)

			errs := validateZoneFields(zone)
			Expect(errs).To(ContainElement(And(
				HaveField("Field", path),
				HaveField("Detail", ContainSubstring("sole identity provider")),
			)))
		},
		Entry("from a gateway", func(z *adminv1.Zone) { z.Spec.Gateways[0].Admin.IdentityProviderRef = "secondary" }, "spec.gateways[0].admin.identityProviderRef"),
		Entry("from a preset", func(z *adminv1.Zone) { z.Spec.Presets[0].IdentityProviderRef = "secondary" }, "spec.presets[0].identityProviderRef"),
	)

	DescribeTable("rejects removal or rename of referenced names",
		func(mutate func(*adminv1.Zone), message string) {
			oldZone := validZone()
			newZone := validZone()
			mutate(newZone)
			_, err := validator.ValidateUpdate(ctx, oldZone, newZone)
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		},
		Entry("gateway rename", func(z *adminv1.Zone) { z.Spec.Gateways[0].Name = "renamed" }, "gateway name"),
		Entry("IDP rename", func(z *adminv1.Zone) { z.Spec.IdentityProviders[0].Name = "renamed" }, "identity provider name"),
	)

	It("allows gateway reordering and prepending a new gateway", func() {
		oldZone := validZone()
		second := oldZone.Spec.Gateways[0]
		second.Name = "secondary"
		oldZone.Spec.Gateways = append(oldZone.Spec.Gateways, second)
		newZone := oldZone.DeepCopy()
		prepended := second
		prepended.Name = "prepended"
		newZone.Spec.Gateways = []adminv1.GatewayConfig{prepended, second, oldZone.Spec.Gateways[0]}

		_, err := validator.ValidateUpdate(ctx, oldZone, newZone)
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("rejects duplicate names on create and update",
		func(mutate func(*adminv1.Zone), message string) {
			zone := validZone()
			mutate(zone)
			_, err := validator.ValidateCreate(ctx, zone)
			Expect(err).To(MatchError(ContainSubstring(message)))

			_, err = validator.ValidateUpdate(ctx, validZone(), zone)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("gateway", func(z *adminv1.Zone) {
			z.Spec.Gateways = append(z.Spec.Gateways, z.Spec.Gateways[0])
		}, "duplicate gateway name"),
		Entry("identity provider", func(z *adminv1.Zone) {
			z.Spec.IdentityProviders = append(z.Spec.IdentityProviders, z.Spec.IdentityProviders[0])
		}, "duplicate identity provider name"),
	)

	DescribeTable("rejects invalid feature placement",
		func(mutate func(*adminv1.Zone), message string) {
			zone := validZone()
			mutate(zone)
			_, err := validator.ValidateCreate(ctx, zone)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("zone feature on preset", func(z *adminv1.Zone) {
			z.Spec.Presets[0].Features = []adminv1.Feature{{Name: adminv1.FeatureBasicAuth, Enabled: true}}
		}, "zone-scoped"),
		Entry("preset feature on zone", func(z *adminv1.Zone) {
			z.Spec.Features = []adminv1.Feature{{Name: adminv1.FeatureAiGateway, Enabled: true}}
		}, "preset-scoped"),
		Entry("unknown zone feature", func(z *adminv1.Zone) {
			z.Spec.Features = []adminv1.Feature{{Name: adminv1.FeatureName("Unknown"), Enabled: true}}
		}, "unknown feature"),
		Entry("duplicate zone feature", func(z *adminv1.Zone) {
			z.Spec.Features = []adminv1.Feature{
				{Name: adminv1.FeatureBasicAuth, Enabled: true},
				{Name: adminv1.FeatureBasicAuth, Enabled: false},
			}
		}, "duplicate feature"),
	)

	It("allows deletion", func() {
		warnings, err := validator.ValidateDelete(ctx, validZone())
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeEmpty())
	})

	DescribeTable("validates configured URLs",
		func(rawURL string, valid bool) {
			errs := validateHTTPSURL(field.NewPath("spec", "url"), rawURL)
			if valid {
				Expect(errs).To(BeEmpty())
			} else {
				Expect(errs).NotTo(BeEmpty())
			}
		},
		Entry("public host", "https://login.example.com/auth/realms/demo?client=rover", true),
		Entry("cluster-local host", "https://keycloak.identity.svc.cluster.local/auth/admin/realms", true),
		Entry("localhost", "https://localhost:8443/admin", true),
		Entry("IPv4 literal", "https://10.0.0.8:8443/admin", true),
		Entry("HTTP", "http://login.example.com/token", false),
		Entry("relative", "/token", false),
		Entry("userinfo", "https://user:pass@login.example.com/token", false),
		Entry("fragment", "https://login.example.com/token#fragment", false),
		Entry("bad port", "https://login.example.com:invalid/token", false),
		Entry("out-of-range port", "https://login.example.com:65536/token", false),
		Entry("malformed escape", "https://login.example.com/%zz", false),
		Entry("control character", "https://login.example.com/a\nb", false),
	)

	DescribeTable("rejects queries in persisted token URLs",
		func(mutate func(*adminv1.Zone), path string) {
			zone := validZone()
			mutate(zone)
			Expect(validateZoneFields(zone)).To(ContainElement(HaveField("Field", path)))
		},
		Entry("preset token URL", func(z *adminv1.Zone) {
			z.Spec.Presets[0].TokenUrl = "https://login.example.com/token?client=rover"
		}, "spec.presets[0].tokenUrl"),
		Entry("identity provider token URL", func(z *adminv1.Zone) {
			z.Spec.IdentityProviders[0].TokenUrl = "https://login.example.com/token?client=rover"
		}, "spec.identityProviders[0].tokenUrl"),
	)

	DescribeTable("validates configured hostnames",
		func(hostname string, valid bool) {
			errs := validateHostname(field.NewPath("spec", "hostname"), hostname)
			if valid {
				Expect(errs).To(BeEmpty())
			} else {
				Expect(errs).NotTo(BeEmpty())
			}
		},
		Entry("public host", "login.example.com", true),
		Entry("localhost", "localhost", true),
		Entry("IPv4 literal", "10.0.0.8", true),
		Entry("cluster-local host", "keycloak.identity.svc.cluster.local", true),
		Entry("scheme", "https://login.example.com", false),
		Entry("port", "login.example.com:8443", false),
		Entry("wildcard", "*.example.com", false),
		Entry("path", "login.example.com/path", false),
		Entry("userinfo", "user@login.example.com", false),
		Entry("query", "login.example.com?x=1", false),
		Entry("fragment", "login.example.com#fragment", false),
	)

	DescribeTable("validates configured base paths",
		func(basePath string, valid bool) {
			errs := validateBasePath(field.NewPath("spec", "basePath"), basePath)
			if valid {
				Expect(errs).To(BeEmpty())
			} else {
				Expect(errs).NotTo(BeEmpty())
			}
		},
		Entry("root", "/", true),
		Entry("nested", "/api/v1", true),
		Entry("relative", "api/v1", false),
		Entry("query", "/api?x=1", false),
		Entry("fragment", "/api#fragment", false),
	)

	DescribeTable("rejects invalid URL fields through admission",
		func(mutate func(*adminv1.Zone), message string) {
			zone := validZone()
			mutate(zone)
			_, err := validator.ValidateCreate(ctx, zone)
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		},
		Entry("gateway admin HTTP URL", func(z *adminv1.Zone) { z.Spec.Gateways[0].Admin.Url = "http://gateway.example.com" }, "admin.url"),
		Entry("IDP admin HTTP URL", func(z *adminv1.Zone) { *z.Spec.IdentityProviders[0].Admin.Url = "http://idp.example.com" }, "admin.url"),
		Entry("IDP token HTTP URL", func(z *adminv1.Zone) { z.Spec.IdentityProviders[0].TokenUrl = "http://idp.example.com/token" }, "tokenUrl"),
		Entry("preset token HTTP URL", func(z *adminv1.Zone) { z.Spec.Presets[0].TokenUrl = "http://idp.example.com/token" }, "tokenUrl"),
		Entry("invalid issuer hostname", func(z *adminv1.Zone) { z.Spec.IdentityProviders[0].IssuerHostname = "https://idp.example.com" }, "issuerHostname"),
		Entry("invalid preset hostname", func(z *adminv1.Zone) { z.Spec.Presets[0].Urls[0].Hostname = "api.example.com:8443" }, "hostname"),
		Entry("invalid preset base path", func(z *adminv1.Zone) { z.Spec.Presets[0].Urls[0].BasePath = "/api?x=1" }, "basePath"),
		Entry("hidden-only URL list", func(z *adminv1.Zone) { z.Spec.Presets[0].Urls[0].Hidden = true }, "non-hidden URL"),
	)

	It("requires an admin URL hostname when issuerHostname is absent", func() {
		zone := validZone()
		zone.Spec.IdentityProviders[0].IssuerHostname = ""
		zone.Spec.IdentityProviders[0].Admin.Url = nil
		_, err := validator.ValidateCreate(ctx, zone)
		Expect(err).To(MatchError(ContainSubstring("issuerHostname")))
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
	})

	It("does not require issuerHostname when an invalid-scheme admin URL has a usable hostname", func() {
		zone := validZone()
		zone.Spec.IdentityProviders[0].IssuerHostname = ""
		adminURL := "http://idp.example.com/admin"
		zone.Spec.IdentityProviders[0].Admin.Url = &adminURL

		_, err := validator.ValidateCreate(ctx, zone)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("admin.url"))
		Expect(err.Error()).NotTo(ContainSubstring("issuerHostname"))
	})
})
