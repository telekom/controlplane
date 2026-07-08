// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
)

var _ = Describe("Feature Priority Ordering", func() {

	DescribeTable("assigns correct absolute priority values to all features",
		func(f features.Feature, expectedPriority int) {
			Expect(f.Priority()).To(Equal(expectedPriority))
		},
		Entry("PassThrough has priority 0",
			features.Feature(feature.InstancePassThroughFeature), 0),
		Entry("HeaderTransformation has priority 0",
			features.Feature(feature.InstanceHeaderTransformationFeature), 0),
		Entry("ExternalIDP has priority 9",
			features.Feature(feature.InstanceExternalIDPFeature), 9),
		Entry("AccessControl has priority 10",
			features.Feature(feature.InstanceAccessControlFeature), 10),
		Entry("CustomScopes has priority 10",
			features.Feature(feature.InstanceCustomScopesFeature), 10),
		Entry("RateLimit has priority 10",
			features.Feature(feature.InstanceRateLimitFeature), 10),
		Entry("BasicAuth has priority 10",
			features.Feature(feature.InstanceBasicAuthFeature), 10),
		Entry("IpRestriction has priority 10",
			features.Feature(feature.InstanceIpRestrictionFeature), 10),
		Entry("LastMileSecurity has priority 100",
			features.Feature(feature.InstanceLastMileSecurityFeature), 100),
		Entry("DynamicUpstream has priority 101",
			features.Feature(feature.InstanceDynamicUpstreamFeature), 101),
		Entry("LoadBalancing has priority 102",
			features.Feature(feature.InstanceLoadBalancingFeature), 102),
		Entry("Failover has priority 109",
			features.Feature(feature.InstanceFailoverFeature), 109),
		Entry("CircuitBreaker has priority 110",
			features.Feature(feature.InstanceCircuitBreakerFeature), 110),
	)

	It("ensures ExternalIDP runs before CustomScopes", func() {
		Expect(feature.InstanceExternalIDPFeature.Priority()).To(
			BeNumerically("<", feature.InstanceCustomScopesFeature.Priority()),
		)
	})

	It("ensures LastMileSecurity runs before DynamicUpstream", func() {
		Expect(feature.InstanceLastMileSecurityFeature.Priority()).To(
			BeNumerically("<", feature.InstanceDynamicUpstreamFeature.Priority()),
		)
	})

	It("ensures DynamicUpstream runs before LoadBalancing", func() {
		Expect(feature.InstanceDynamicUpstreamFeature.Priority()).To(
			BeNumerically("<", feature.InstanceLoadBalancingFeature.Priority()),
		)
	})

	It("ensures LoadBalancing runs before Failover", func() {
		Expect(feature.InstanceLoadBalancingFeature.Priority()).To(
			BeNumerically("<", feature.InstanceFailoverFeature.Priority()),
		)
	})

	It("ensures Failover runs before CircuitBreaker", func() {
		Expect(feature.InstanceFailoverFeature.Priority()).To(
			BeNumerically("<", feature.InstanceCircuitBreakerFeature.Priority()),
		)
	})

	It("sorts features correctly when using sortFeatures-like logic", func() {
		allFeatures := []features.Feature{
			feature.InstanceCircuitBreakerFeature,
			feature.InstanceFailoverFeature,
			feature.InstanceLoadBalancingFeature,
			feature.InstanceDynamicUpstreamFeature,
			feature.InstanceLastMileSecurityFeature,
			feature.InstanceIpRestrictionFeature,
			feature.InstanceBasicAuthFeature,
			feature.InstanceRateLimitFeature,
			feature.InstanceCustomScopesFeature,
			feature.InstanceAccessControlFeature,
			feature.InstanceExternalIDPFeature,
			feature.InstanceHeaderTransformationFeature,
			feature.InstancePassThroughFeature,
		}

		sort.Slice(allFeatures, func(i, j int) bool {
			return allFeatures[i].Priority() < allFeatures[j].Priority()
		})

		// Verify the sorted order by priority groups
		// Priority 0: PassThrough, HeaderTransformation (order within same priority is undefined)
		Expect(allFeatures[0].Priority()).To(Equal(0))
		Expect(allFeatures[1].Priority()).To(Equal(0))

		// Priority 9: ExternalIDP
		Expect(allFeatures[2].Priority()).To(Equal(9))
		Expect(allFeatures[2].Name()).To(Equal(feature.InstanceExternalIDPFeature.Name()))

		// Priority 10: AccessControl, CustomScopes, RateLimit, BasicAuth, IpRestriction
		for i := 3; i <= 7; i++ {
			Expect(allFeatures[i].Priority()).To(Equal(10))
		}

		// Priority 100: LastMileSecurity
		Expect(allFeatures[8].Priority()).To(Equal(100))
		Expect(allFeatures[8].Name()).To(Equal(feature.InstanceLastMileSecurityFeature.Name()))

		// Priority 101: DynamicUpstream
		Expect(allFeatures[9].Priority()).To(Equal(101))
		Expect(allFeatures[9].Name()).To(Equal(feature.InstanceDynamicUpstreamFeature.Name()))

		// Priority 102: LoadBalancing
		Expect(allFeatures[10].Priority()).To(Equal(102))
		Expect(allFeatures[10].Name()).To(Equal(feature.InstanceLoadBalancingFeature.Name()))

		// Priority 109: Failover
		Expect(allFeatures[11].Priority()).To(Equal(109))
		Expect(allFeatures[11].Name()).To(Equal(feature.InstanceFailoverFeature.Name()))

		// Priority 110: CircuitBreaker
		Expect(allFeatures[12].Priority()).To(Equal(110))
		Expect(allFeatures[12].Name()).To(Equal(feature.InstanceCircuitBreakerFeature.Name()))
	})
})
