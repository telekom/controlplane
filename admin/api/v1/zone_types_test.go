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
				{Name: "standard", Admin: GatewayAdminConfig{IdentityProviderRef: "primary"}},
				{Name: "ai", Admin: GatewayAdminConfig{IdentityProviderRef: "primary"}},
			},
			IdentityProviders: []IdentityProviderConfig{{Name: "primary"}},
			Presets: []Preset{
				{Name: "default", Type: GatewayTypeAPI, Default: true, GatewayRef: "standard", IdentityProviderRef: "primary"},
				{Name: "failover", Type: GatewayTypeAPI, GatewayRef: "standard", IdentityProviderRef: "primary", Features: []Feature{{Name: FeatureConsumerFailover, Enabled: true}}},
				{Name: "ai", Type: GatewayTypeAI, Default: true, GatewayRef: "ai", IdentityProviderRef: "primary"},
			},
		}
	}

	It("prefers the default preset when it serves the gateway type", func() {
		preset, err := zoneSpec().SelectPreset(GatewayTypeAPI)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
	})

	It("returns the type default when no preset-scoped features are requested", func() {
		preset, err := zoneSpec().SelectPreset(GatewayTypeAI)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("ai"))
	})

	It("reports a traffic type that has no preset", func() {
		_, err := zoneSpec().SelectPreset(GatewayTypeEvent)
		Expect(err).To(MatchError(ContainSubstring(`gateway type "Event": no preset of type "Event" exists`)))
		Expect(errors.Is(err, ErrNoMatchingPreset)).To(BeTrue())
	})

	It("omits the feature clause when no features were requested", func() {
		_, err := zoneSpec().SelectPreset(GatewayTypeEvent)
		Expect(err.Error()).NotTo(ContainSubstring("feature combination"))
	})

	It("reports a feature that no preset of the type enables", func() {
		spec := zoneSpec()
		_, err := spec.SelectPreset(GatewayTypeAI, FeatureConsumerFailover)
		Expect(err).To(MatchError(ContainSubstring(`feature "ConsumerFailover" is not enabled on any "AI" preset`)))
	})

	It("uses the first preset of a type when none is default", func() {
		spec := zoneSpec()
		spec.Presets[2].Default = false
		preset, err := spec.SelectPreset(GatewayTypeAI)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("ai"))
	})

	It("prefers the default when several presets match a feature", func() {
		spec := zoneSpec()
		spec.Presets[0].Features = []Feature{{Name: FeatureConsumerFailover, Enabled: true}}
		spec.Presets = append(spec.Presets, Preset{
			Name: "second-failover", Type: GatewayTypeAPI, GatewayRef: "standard", IdentityProviderRef: "primary",
			Features: []Feature{{Name: FeatureConsumerFailover, Enabled: true}},
		})

		preset, err := spec.SelectPreset(GatewayTypeAPI, FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
	})

	It("rejects a type carrying more than one default preset", func() {
		spec := zoneSpec()
		spec.Presets[1].Default = true

		_, err := spec.SelectPreset(GatewayTypeAPI)
		Expect(err).To(MatchError(ContainSubstring(`2 presets of type "API" are marked default; exactly one is required`)))
		Expect(errors.Is(err, ErrAmbiguousPreset)).To(BeTrue())
		Expect(errors.Is(err, ErrNoMatchingPreset)).To(BeFalse())
	})

	It("distinguishes an invalid feature from an unavailable one", func() {
		spec := zoneSpec()

		_, err := spec.SelectPreset(GatewayTypeAPI, FeatureName("Unknown"))
		Expect(errors.Is(err, ErrInvalidFeatures)).To(BeTrue())
		Expect(errors.Is(err, ErrNoMatchingPreset)).To(BeFalse())

		_, err = spec.MatchingGateways(GatewayTypeAPI, FeatureName("Unknown"))
		Expect(errors.Is(err, ErrInvalidFeatures)).To(BeTrue())
		Expect(errors.Is(err, ErrNoMatchingPreset)).To(BeFalse())
	})

	It("resolves GetDefaultPreset to the API type default", func() {
		preset, err := zoneSpec().GetDefaultPreset()
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
		Expect(preset.Type).To(Equal(GatewayTypeAPI))
	})

	It("returns distinct gateways for a type and feature", func() {
		spec := zoneSpec()
		spec.Presets = append(spec.Presets, Preset{
			Name: "ai-failover", Type: GatewayTypeAI, GatewayRef: "ai", IdentityProviderRef: "primary",
			Features: []Feature{{Name: FeatureConsumerFailover, Enabled: true}},
		})

		gateways, err := spec.MatchingGateways(GatewayTypeAI, FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		Expect(gateways).To(HaveLen(1))
		Expect(gateways[0].Name).To(Equal("ai"))
	})

	It("resolves each traffic type on a shared gateway to its own preset", func() {
		spec := zoneSpec()
		spec.Presets[2].GatewayRef = "standard"
		spec.Presets = append(spec.Presets, Preset{
			Name: "ai-failover", Type: GatewayTypeAI, GatewayRef: "standard", IdentityProviderRef: "primary",
			Features: []Feature{{Name: FeatureConsumerFailover, Enabled: true}},
		})

		api, err := spec.SelectPreset(GatewayTypeAPI)
		Expect(err).NotTo(HaveOccurred())
		Expect(api.Name).To(Equal("default"))

		apiFailover, err := spec.SelectPreset(GatewayTypeAPI, FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		Expect(apiFailover.Name).To(Equal("failover"))

		ai, err := spec.SelectPreset(GatewayTypeAI)
		Expect(err).NotTo(HaveOccurred())
		Expect(ai.Name).To(Equal("ai"))

		aiFailover, err := spec.SelectPreset(GatewayTypeAI, FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		Expect(aiFailover.Name).To(Equal("ai-failover"))
	})

	It("returns the single gateway serving a type and feature", func() {
		spec := zoneSpec()
		spec.Presets = append(spec.Presets,
			Preset{Name: "same-gateway", Type: GatewayTypeAI, GatewayRef: "standard", IdentityProviderRef: "primary", Features: []Feature{{Name: FeatureConsumerFailover, Enabled: true}}},
		)

		gateways, err := spec.MatchingGateways(GatewayTypeAI, FeatureConsumerFailover)
		Expect(err).NotTo(HaveOccurred())
		Expect(gateways).To(HaveLen(1))
		Expect(gateways[0].Name).To(Equal("standard"))
	})

	It("reports invalid features before broken gateway references", func() {
		spec := zoneSpec()
		spec.Presets[0].GatewayRef = "missing"

		gateways, err := spec.MatchingGateways(GatewayTypeAPI, FeatureName("Unknown"))
		Expect(err).To(MatchError(ContainSubstring(`feature "Unknown" is unknown`)))
		Expect(err).NotTo(MatchError(ContainSubstring(`gateway "missing" not found`)))
		Expect(gateways).To(BeNil())
	})

	It("ignores disabled failover features", func() {
		spec := zoneSpec()
		spec.Presets[1].Features[0].Enabled = false

		gateways, err := spec.MatchingGateways(GatewayTypeAPI, FeatureConsumerFailover)
		Expect(err).To(MatchError(ContainSubstring(`feature "ConsumerFailover" is not enabled on any "API" preset`)))
		Expect(errors.Is(err, ErrNoMatchingPreset)).To(BeTrue())
		Expect(gateways).To(BeNil())
	})

	It("does not classify a broken matching gateway reference as no match", func() {
		spec := zoneSpec()
		spec.Presets[1].GatewayRef = "missing"

		gateways, err := spec.MatchingGateways(GatewayTypeAPI, FeatureConsumerFailover)

		Expect(err).To(MatchError(ContainSubstring(`gateway "missing" not found`)))
		Expect(errors.Is(err, ErrNoMatchingPreset)).To(BeFalse())
		Expect(gateways).To(BeNil())
	})

	It("inherits zone features", func() {
		preset, err := zoneSpec().SelectPreset(GatewayTypeAPI, FeatureBasicAuth)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
		Expect(zoneSpec().FeaturesSupported(GatewayTypeAPI, FeatureBasicAuth)).To(BeTrue())
	})

	It("allows presets to override inherited features", func() {
		spec := zoneSpec()
		spec.Presets[0].Features = []Feature{{Name: FeatureBasicAuth, Enabled: false}}
		spec.Presets[1].Features = append(spec.Presets[1].Features, Feature{Name: FeatureBasicAuth, Enabled: true})

		preset, err := spec.SelectPreset(GatewayTypeAPI, FeatureBasicAuth)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("failover"))
	})

	It("allows a preset to enable a zone-disabled feature", func() {
		spec := zoneSpec()
		spec.Features[0].Enabled = false
		spec.Presets[0].Features = []Feature{{Name: FeatureBasicAuth, Enabled: true}}

		preset, err := spec.SelectPreset(GatewayTypeAPI, FeatureBasicAuth)
		Expect(err).NotTo(HaveOccurred())
		Expect(preset.Name).To(Equal("default"))
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
		Expect(err).To(MatchError(ContainSubstring(`feature combination [BasicAuth Unknown ConsumerFailover] for gateway type "API"`)))
		Expect(err).To(MatchError(ContainSubstring(`feature "BasicAuth" is not enabled on any "API" preset`)))
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
