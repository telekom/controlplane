// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/projector/internal/domain/eventtype"
	"github.com/telekom/controlplane/projector/internal/domain/group"
	"github.com/telekom/controlplane/projector/internal/domain/permissionset"
	"github.com/telekom/controlplane/projector/internal/domain/team"
	"github.com/telekom/controlplane/projector/internal/domain/zone"
	"github.com/telekom/controlplane/projector/internal/module"
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
		baseModules        []module.Module
	)

	BeforeEach(func() {
		originalPermission = cconfig.FeaturePermission.IsEnabled()
		originalPubSub = cconfig.FeaturePubSub.IsEnabled()
		baseModules = []module.Module{zone.Module, group.Module, team.Module}
	})

	AfterEach(func() {
		cconfig.SetFeatureEnabled(cconfig.FeaturePermission, originalPermission)
		cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, originalPubSub)
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

	DescribeTable("feature flag matrix",
		func(pubSubEnabled, permissionEnabled bool) {
			cconfig.SetFeatureEnabled(cconfig.FeaturePubSub, pubSubEnabled)
			cconfig.SetFeatureEnabled(cconfig.FeaturePermission, permissionEnabled)

			result := registerSchemesAndModules(runtime.NewScheme(), baseModules)

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

			// baseModules must not be mutated by the append inside registerSchemesAndModules.
			Expect(baseModules).To(HaveLen(3))
			Expect(moduleNames(baseModules)).To(Equal([]string{zone.Module.Name(), group.Module.Name(), team.Module.Name()}))
		},
		Entry("both disabled", false, false),
		Entry("pubsub only", true, false),
		Entry("permission only", false, true),
		Entry("both enabled", true, true),
	)
})
