// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestZoneTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Zone Types Suite")
}

var _ = Describe("Zone preset resolution", func() {
	zoneSpec := func() *ZoneSpec {
		return &ZoneSpec{
			Features: []Feature{{Name: FeatureBasicAuth, Enabled: true}},
			Gateways: []GatewayConfig{
				{
					Name:  "standard",
					Types: []GatewayType{GatewayTypeAPI, GatewayTypeEvent},
					Admin: GatewayAdminConfig{IdentityProviderRef: "primary"},
				},
				{
					Name:  "ai",
					Types: []GatewayType{GatewayTypeAI},
					Admin: GatewayAdminConfig{IdentityProviderRef: "primary"},
				},
			},
			IdentityProviders: []IdentityProviderConfig{{Name: "primary"}},
			Presets: []Preset{
				{Name: "default", Default: true, GatewayRef: "standard", IdentityProviderRef: "primary"},
				{Name: "failover", GatewayRef: "standard", IdentityProviderRef: "primary", Features: []Feature{{Name: FeatureConsumerFailover, Enabled: true}}},
				{Name: "ai", GatewayRef: "ai", IdentityProviderRef: "primary"},
			},
		}
	}

	It("resolves the default preset", func() {
		preset, err := zoneSpec().GetDefaultPreset()
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
	})

	It("selects presets by gateway type", func() {
		preset, err := zoneSpec().SelectPreset(GatewayTypeAI)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("ai"))
	})

	It("prefers the default preset when it serves the gateway type", func() {
		preset, err := zoneSpec().SelectPreset(GatewayTypeAPI)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
	})

	It("reports gateway types no preset serves", func() {
		spec := zoneSpec()
		spec.Gateways[1].Types = []GatewayType{GatewayTypeAPI}
		_, err := spec.SelectPreset(GatewayTypeAI)
		Expect(err).To(MatchError(ContainSubstring(`no preset references a gateway of type "AI"`)))
	})

	It("takes the first match when several presets carry the same features", func() {
		spec := zoneSpec()
		spec.Presets = append(spec.Presets, Preset{
			Name: "second-failover", GatewayRef: "standard", IdentityProviderRef: "primary",
			Features: []Feature{{Name: FeatureConsumerFailover, Enabled: true}},
		})
		preset, err := spec.SelectPreset(GatewayTypeAPI, FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("failover"))
	})

	It("returns the default preset for zone-only features", func() {
		preset, err := zoneSpec().SelectPreset(GatewayTypeAPI, FeatureBasicAuth)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
		Expect(zoneSpec().FeaturesSupported(GatewayTypeAPI, FeatureBasicAuth)).To(BeTrue())
	})

	It("combines zone and preset features", func() {
		preset, err := zoneSpec().SelectPreset(GatewayTypeAPI, FeatureBasicAuth, FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("failover"))
		Expect(zoneSpec().FeaturesSupported(GatewayTypeAPI, FeatureBasicAuth, FeatureConsumerFailover)).To(BeTrue())
	})

	It("reports every reason a feature combination is not allowed", func() {
		spec := zoneSpec()
		spec.Features[0].Enabled = false
		spec.Presets[1].Features[0].Enabled = false

		_, err := spec.SelectPreset(GatewayTypeAPI, FeatureBasicAuth, FeatureName("Unknown"), FeatureConsumerFailover)
		Expect(err).To(MatchError(ContainSubstring("feature combination")))
		Expect(err).To(MatchError(ContainSubstring("is not allowed for gateway type \"API\"")))
		Expect(err).To(MatchError(ContainSubstring(`zone feature "BasicAuth" is not enabled`)))
		Expect(err).To(MatchError(ContainSubstring(`feature "Unknown" is unknown`)))
		Expect(err).To(MatchError(ContainSubstring(`feature "ConsumerFailover" is not enabled on any "API" preset`)))
	})

	It("rejects disabled and unknown zone features", func() {
		spec := zoneSpec()
		spec.Features[0].Enabled = false
		Expect(spec.FeaturesSupported(GatewayTypeAPI, FeatureBasicAuth)).To(BeFalse())
		Expect(spec.FeaturesSupported(GatewayTypeAPI, FeatureName("Unknown"))).To(BeFalse())
	})

	It("does not treat disabled features as supported", func() {
		spec := zoneSpec()
		spec.Presets[1].Features[0].Enabled = false
		_, err := spec.SelectPreset(GatewayTypeAPI, FeatureConsumerFailover)
		Expect(err).To(MatchError(ContainSubstring(`not enabled on any "API" preset`)))
	})

	It("requires exactly one default preset", func() {
		spec := zoneSpec()
		spec.Presets[1].Default = true
		_, err := spec.GetDefaultPreset()
		Expect(err).To(MatchError(ContainSubstring("multiple default presets")))
	})

	It("returns pointers to the stored entries", func() {
		spec := zoneSpec()
		gateway, err := spec.GetGateway("standard")
		Expect(err).NotTo(HaveOccurred())
		gateway.Name = "updated"
		Expect(spec.Gateways[0].Name).To(Equal("updated"))

		idp, err := spec.GetIdentityProviderByName("primary")
		Expect(err).NotTo(HaveOccurred())
		idp.Name = "updated"
		Expect(spec.IdentityProviders[0].Name).To(Equal("updated"))

		preset, err := spec.GetPreset("default")
		Expect(err).NotTo(HaveOccurred())
		preset.Name = "updated"
		Expect(spec.Presets[0].Name).To(Equal("updated"))
	})

	It("returns useful not-found errors", func() {
		spec := zoneSpec()
		_, err := spec.GetGateway("missing")
		Expect(err).To(MatchError(ContainSubstring("gateway")))
		_, err = spec.GetIdentityProviderByName("missing")
		Expect(err).To(MatchError(ContainSubstring("identity provider")))
		_, err = spec.GetPreset("missing")
		Expect(errors.Is(err, ErrNoPresetFound)).To(BeTrue())

		status := ZoneStatus{}
		_, err = status.GetGateway("missing")
		Expect(err).To(MatchError(ContainSubstring("gateway status")))
		_, err = status.GetPreset("missing")
		Expect(err).To(MatchError(ContainSubstring("preset status")))
	})

	It("returns the sole identity provider", func() {
		spec := zoneSpec()
		identityProvider, err := spec.GetIdentityProvider()
		Expect(err).NotTo(HaveOccurred())
		Expect(identityProvider).To(BeIdenticalTo(&spec.IdentityProviders[0]))

		spec.IdentityProviders = nil
		_, err = spec.GetIdentityProvider()
		Expect(err).To(MatchError(ContainSubstring("exactly one identity provider")))

		spec.IdentityProviders = []IdentityProviderConfig{{Name: "one"}, {Name: "two"}}
		_, err = spec.GetIdentityProvider()
		Expect(err).To(MatchError(ContainSubstring("exactly one identity provider")))
	})

	It("looks up stored status entries", func() {
		status := ZoneStatus{
			Gateways: []GatewayStatus{{Name: "standard"}},
			Presets:  []PresetStatus{{Name: "default"}},
		}

		gateway, err := status.GetGateway("standard")
		Expect(err).NotTo(HaveOccurred())
		gateway.Name = "updated"
		Expect(status.Gateways[0].Name).To(Equal("updated"))

		preset, err := status.GetPreset("default")
		Expect(err).NotTo(HaveOccurred())
		preset.Name = "updated"
		Expect(status.Presets[0].Name).To(Equal("updated"))
	})
})
