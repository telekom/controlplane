// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
)

var _ = Describe("computeStrategy", func() {
	It("should return Auto when both teams are the same", func() {
		Expect(computeStrategy("teamA", "teamA")).To(Equal(approvalapi.ApprovalStrategyAuto))
	})

	It("should return Simple when teams differ", func() {
		Expect(computeStrategy("teamA", "teamB")).To(Equal(approvalapi.ApprovalStrategySimple))
	})

	It("should return Simple when both teams are empty", func() {
		Expect(computeStrategy("", "")).To(Equal(approvalapi.ApprovalStrategySimple))
	})

	It("should return Simple when requester team is empty", func() {
		Expect(computeStrategy("", "teamA")).To(Equal(approvalapi.ApprovalStrategySimple))
	})

	It("should return Simple when decider team is empty", func() {
		Expect(computeStrategy("teamA", "")).To(Equal(approvalapi.ApprovalStrategySimple))
	})
})

var _ = Describe("computeAggregateOutcome", func() {
	// Full pair table per spec.

	It("Both Granted -> outcomeGranted", func() {
		Expect(computeAggregateOutcome(outcomeGranted, outcomeGranted)).To(Equal(outcomeGranted))
	})

	It("Provider Granted + Consumer Pending -> outcomePending", func() {
		Expect(computeAggregateOutcome(outcomeGranted, outcomePending)).To(Equal(outcomePending))
	})

	It("Provider Pending + Consumer Granted -> outcomePending", func() {
		Expect(computeAggregateOutcome(outcomePending, outcomeGranted)).To(Equal(outcomePending))
	})

	It("Provider Denied + Consumer Granted -> outcomeDenied", func() {
		Expect(computeAggregateOutcome(outcomeDenied, outcomeGranted)).To(Equal(outcomeDenied))
	})

	It("Provider Granted + Consumer Denied -> outcomeDenied", func() {
		Expect(computeAggregateOutcome(outcomeGranted, outcomeDenied)).To(Equal(outcomeDenied))
	})

	It("Provider Error + Consumer Pending -> outcomeError (NOT outcomePending)", func() {
		Expect(computeAggregateOutcome(outcomeError, outcomePending)).To(Equal(outcomeError))
	})

	It("Both Pending -> outcomePending", func() {
		Expect(computeAggregateOutcome(outcomePending, outcomePending)).To(Equal(outcomePending))
	})

	It("Both Error -> outcomeError", func() {
		Expect(computeAggregateOutcome(outcomeError, outcomeError)).To(Equal(outcomeError))
	})

	It("Provider Denied + Consumer Error -> outcomeDenied (denial takes precedence over error for cleanup)", func() {
		Expect(computeAggregateOutcome(outcomeDenied, outcomeError)).To(Equal(outcomeDenied))
	})

	It("Consumer Denied + Provider Error -> outcomeDenied", func() {
		Expect(computeAggregateOutcome(outcomeError, outcomeDenied)).To(Equal(outcomeDenied))
	})

	// outcomeUnknown (zero value) never grants.

	It("Provider Unknown + Consumer Granted -> outcomeError", func() {
		Expect(computeAggregateOutcome(outcomeUnknown, outcomeGranted)).To(Equal(outcomeError))
	})

	It("Both Unknown -> outcomeError", func() {
		Expect(computeAggregateOutcome(outcomeUnknown, outcomeUnknown)).To(Equal(outcomeError))
	})

	It("Provider Granted + Consumer Unknown -> outcomeError", func() {
		Expect(computeAggregateOutcome(outcomeGranted, outcomeUnknown)).To(Equal(outcomeError))
	})

	It("Provider Unknown + Consumer Denied -> outcomeDenied (denial still takes precedence)", func() {
		Expect(computeAggregateOutcome(outcomeUnknown, outcomeDenied)).To(Equal(outcomeDenied))
	})

	// RequestDenied combinations.

	It("Provider RequestDenied + Consumer Granted -> outcomeRequestDenied", func() {
		Expect(computeAggregateOutcome(outcomeRequestDenied, outcomeGranted)).To(Equal(outcomeRequestDenied))
	})

	It("Provider Granted + Consumer RequestDenied -> outcomeRequestDenied", func() {
		Expect(computeAggregateOutcome(outcomeGranted, outcomeRequestDenied)).To(Equal(outcomeRequestDenied))
	})

	It("Provider Denied + Consumer RequestDenied -> outcomeDenied (denial takes precedence over request denial)", func() {
		Expect(computeAggregateOutcome(outcomeDenied, outcomeRequestDenied)).To(Equal(outcomeDenied))
	})

	// Pending + Error is Error, not Pending.

	It("Provider Pending + Consumer Error -> outcomeError", func() {
		Expect(computeAggregateOutcome(outcomePending, outcomeError)).To(Equal(outcomeError))
	})
})

var _ = Describe("approvalOutcome.String", func() {
	It("should return readable names for all outcomes", func() {
		Expect(outcomeUnknown.String()).To(Equal("Unknown"))
		Expect(outcomeGranted.String()).To(Equal("Granted"))
		Expect(outcomePending.String()).To(Equal("Pending"))
		Expect(outcomeRequestDenied.String()).To(Equal("RequestDenied"))
		Expect(outcomeDenied.String()).To(Equal("Denied"))
		Expect(outcomeError.String()).To(Equal("Error"))
	})
})

var _ = Describe("five team relationships", func() {
	// The five relationships determine which gates get Auto vs Simple strategy.
	// These are type-level / design tests — they verify the computeStrategy
	// calls that buildScopedGate would make for each team relationship.

	type gateStrategies struct {
		provider approvalapi.ApprovalStrategy
		consumer approvalapi.ApprovalStrategy
	}

	computeGateStrategies := func(observerTeam, consumerTeam, providerTeam string) gateStrategies {
		return gateStrategies{
			provider: computeStrategy(observerTeam, providerTeam),
			consumer: computeStrategy(observerTeam, consumerTeam),
		}
	}

	It("All teams equal (A==C==P): both Auto", func() {
		gs := computeGateStrategies("teamX", "teamX", "teamX")
		Expect(gs.provider).To(Equal(approvalapi.ApprovalStrategyAuto))
		Expect(gs.consumer).To(Equal(approvalapi.ApprovalStrategyAuto))
	})

	It("A shares only C's team (A==C, A!=P): consumer Auto, provider Simple", func() {
		gs := computeGateStrategies("teamA", "teamA", "teamP")
		Expect(gs.consumer).To(Equal(approvalapi.ApprovalStrategyAuto))
		Expect(gs.provider).To(Equal(approvalapi.ApprovalStrategySimple))
	})

	It("A shares only P's team (A==P, A!=C): provider Auto, consumer Simple", func() {
		gs := computeGateStrategies("teamP", "teamC", "teamP")
		Expect(gs.provider).To(Equal(approvalapi.ApprovalStrategyAuto))
		Expect(gs.consumer).To(Equal(approvalapi.ApprovalStrategySimple))
	})

	It("C/P share team but A differs (C==P, A!=C): both Simple", func() {
		gs := computeGateStrategies("teamA", "teamCP", "teamCP")
		Expect(gs.provider).To(Equal(approvalapi.ApprovalStrategySimple))
		Expect(gs.consumer).To(Equal(approvalapi.ApprovalStrategySimple))
	})

	It("All teams differ (A!=C!=P): both Simple", func() {
		gs := computeGateStrategies("teamA", "teamC", "teamP")
		Expect(gs.provider).To(Equal(approvalapi.ApprovalStrategySimple))
		Expect(gs.consumer).To(Equal(approvalapi.ApprovalStrategySimple))
	})
})

var _ = Describe("gateApprovalProperties", func() {
	It("should use the provided action for provider gate", func() {
		intent := buildAuthorizationIntentCompat(baseListener(), baseConsumerApp(), baseProviderApp(), baseSpectreApp())
		props := intent.gateApprovalProperties("listen-provider")
		Expect(props["action"]).To(Equal("listen-provider"))
	})

	It("should use the provided action for consumer gate", func() {
		intent := buildAuthorizationIntentCompat(baseListener(), baseConsumerApp(), baseProviderApp(), baseSpectreApp())
		props := intent.gateApprovalProperties("listen-consumer")
		Expect(props["action"]).To(Equal("listen-consumer"))
	})

	It("should preserve all existing property keys", func() {
		intent := buildAuthorizationIntentCompat(baseListener(), baseConsumerApp(), baseProviderApp(), baseSpectreApp())
		props := intent.gateApprovalProperties("listen-provider")
		Expect(props).To(HaveKey("consumer"))
		Expect(props).To(HaveKey("provider"))
		Expect(props).To(HaveKey("observer"))
		Expect(props).To(HaveKey("listenerApplication"))
		Expect(props).To(HaveKey("apiBasePath"))
		Expect(props).To(HaveKey("policyVersion"))
		Expect(props).To(HaveKey("consumerTeam"))
		Expect(props).To(HaveKey("providerTeam"))
		Expect(props).To(HaveKey("observerTeam"))
	})

	It("should be backward-compatible with approvalProperties", func() {
		intent := buildAuthorizationIntentCompat(baseListener(), baseConsumerApp(), baseProviderApp(), baseSpectreApp())
		oldProps := intent.approvalProperties()
		newProps := intent.gateApprovalProperties("listen-provider")
		Expect(oldProps).To(Equal(newProps))
	})
})
