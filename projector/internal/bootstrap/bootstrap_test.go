// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/projector/internal/domain/agentcard"
	"github.com/telekom/controlplane/projector/internal/domain/eventtype"
	"github.com/telekom/controlplane/projector/internal/domain/group"
	"github.com/telekom/controlplane/projector/internal/domain/listener"
	"github.com/telekom/controlplane/projector/internal/domain/permissionset"
	"github.com/telekom/controlplane/projector/internal/domain/team"
	"github.com/telekom/controlplane/projector/internal/domain/zone"
	"github.com/telekom/controlplane/projector/internal/module"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

func TestBootstrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap Suite")
}

// moduleNames returns the Name() of every module in the slice, for assertions.
func moduleNames(mods []module.Module) []string {
	names := make([]string, len(mods))
	for i, m := range mods {
		names[i] = m.Name()
	}
	return names
}

var _ = Describe("registerSchemesAndModules", func() {
	var (
		originalPermission bool
		originalPubSub     bool
		originalAiGateway  bool
		originalSpectre    bool
		baseModules        []module.Module
	)

	BeforeEach(func() {
		originalPermission = cconfig.FeaturePermission.IsEnabled()
		originalPubSub = cconfig.FeaturePubSub.IsEnabled()
		originalAiGateway = cconfig.FeatureAiGateway.IsEnabled()
		originalSpectre = cconfig.FeatureSpectre.IsEnabled()
		baseModules = []module.Module{zone.Module, group.Module, team.Module}
	})

	AfterEach(func() {
		cconfig.SetFeatureEnabled(cconfig.FeaturePermission, originalPermission)
		cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, originalPubSub)
		cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, originalAiGateway)
		cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, originalSpectre)
	})

	It("should not register the permissionset module when FeaturePermission is disabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeaturePermission, false)
		cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, false)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).NotTo(ContainElement(permissionset.Module.Name()))
	})

	It("should register the permissionset module when FeaturePermission is enabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeaturePermission, true)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).To(ContainElement(permissionset.Module.Name()))
	})

	It("should not register the agentic modules when FeatureAiGateway is disabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, false)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).NotTo(ContainElement(agentcard.Module.Name()))
	})

	It("should register the agentic modules when FeatureAiGateway is enabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, true)

		result := registerSchemesAndModules(runtime.NewScheme(), append([]module.Module{}, baseModules...))

		Expect(moduleNames(result)).To(ContainElements(
			"mcpserver", "agentcard", "agenticexposure", "agenticsubscription",
		))
	})

	It("registers Listener and its scheme only when Spectre is enabled", func() {
		cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, false)
		disabledScheme := runtime.NewScheme()
		Expect(moduleNames(registerSchemesAndModules(disabledScheme, baseModules))).NotTo(ContainElement(listener.Module.Name()))
		Expect(disabledScheme.Recognizes(spectrev1.GroupVersion.WithKind("Listener"))).To(BeFalse())

		cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, true)
		enabledScheme := runtime.NewScheme()
		names := moduleNames(registerSchemesAndModules(enabledScheme, baseModules))
		Expect(names).To(ContainElement(listener.Module.Name()))
		Expect(enabledScheme.Recognizes(spectrev1.GroupVersion.WithKind("Listener"))).To(BeTrue())
		Expect(enabledScheme.Recognizes(spectrev1.GroupVersion.WithKind("SpectreApplication"))).To(BeTrue())
	})

	DescribeTable("feature flag matrix",
		func(pubSubEnabled, permissionEnabled, aiGatewayEnabled, spectreEnabled bool) {
			cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, pubSubEnabled)
			cconfig.SetFeatureEnabled(cconfig.FeaturePermission, permissionEnabled)
			cconfig.SetFeatureEnabled(cconfig.FeatureAiGateway, aiGatewayEnabled)
			cconfig.SetFeatureEnabled(cconfig.FeatureSpectre, spectreEnabled)

			registeredScheme := runtime.NewScheme()
			result := registerSchemesAndModules(registeredScheme, baseModules)

			names := moduleNames(result)
			Expect(names).To(ContainElements(zone.Module.Name(), group.Module.Name(), team.Module.Name()),
				"base modules must always be present")

			if pubSubEnabled {
				Expect(names).To(ContainElement(eventtype.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(eventtype.Module.Name()))
			}

			if permissionEnabled {
				Expect(names).To(ContainElement(permissionset.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(permissionset.Module.Name()))
			}

			if aiGatewayEnabled {
				Expect(names).To(ContainElement(agentcard.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(agentcard.Module.Name()))
			}
			for _, kind := range []string{"McpServer", "AgentCard", "AgenticExposure", "AgenticSubscription"} {
				Expect(registeredScheme.Recognizes(agenticv1.GroupVersion.WithKind(kind))).To(Equal(aiGatewayEnabled))
			}

			if spectreEnabled {
				Expect(names).To(ContainElement(listener.Module.Name()))
			} else {
				Expect(names).NotTo(ContainElement(listener.Module.Name()))
			}
			for _, kind := range []string{"Listener", "SpectreApplication"} {
				Expect(registeredScheme.Recognizes(spectrev1.GroupVersion.WithKind(kind))).To(Equal(spectreEnabled))
			}

			// baseModules must not be mutated by the append inside registerSchemesAndModules.
			Expect(baseModules).To(HaveLen(3))
			Expect(moduleNames(baseModules)).To(Equal([]string{zone.Module.Name(), group.Module.Name(), team.Module.Name()}))
		},
		Entry("all disabled", false, false, false, false),
		Entry("pubsub only", true, false, false, false),
		Entry("permission only", false, true, false, false),
		Entry("ai_gateway only", false, false, true, false),
		Entry("all except spectre", true, true, true, false),
		Entry("spectre only", false, false, false, true),
		Entry("ai_gateway and spectre", false, false, true, true),
		Entry("all enabled", true, true, true, true),
	)
})
