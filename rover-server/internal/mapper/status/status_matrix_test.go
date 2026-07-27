// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/rover-server/internal/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// conditionInput is a compact representation of a Kubernetes condition for test tables.
type conditionInput struct {
	Type               string
	Status             metav1.ConditionStatus
	Reason             string
	Message            string
	ObservedGeneration int64
}

func toConditions(inputs []conditionInput) []metav1.Condition {
	out := make([]metav1.Condition, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, metav1.Condition{
			Type:               in.Type,
			Status:             in.Status,
			Reason:             in.Reason,
			Message:            in.Message,
			ObservedGeneration: in.ObservedGeneration,
		})
	}
	return out
}

// ---------- helpers ----------

const (
	typeReady      = condition.ConditionTypeReady
	typeProcessing = condition.ConditionTypeProcessing
)

// ---------- Matrix tests ----------

// This file provides systematic, table-driven tests that cover every
// Ready reason × Processing condition combination that can appear in
// production.  The goal is a provable mapping from condition tuples to
// (State, ProcessingState, OverallStatus).

var _ = Describe("fillStateInfo matrix", func() {
	// Each entry is a real production scenario identified by the condition
	// tuple a controller would set.
	DescribeTable("Ready reason matrix",
		func(
			conditions []conditionInput,
			objectGeneration int64,
			expectedState api.State,
			expectedPS api.ProcessingState,
			expectedOverall api.OverallStatus,
		) {
			status := &api.Status{}
			fillStateInfo(toConditions(conditions), objectGeneration, status)

			Expect(status.State).To(Equal(expectedState), "State mismatch")
			Expect(status.ProcessingState).To(Equal(expectedPS), "ProcessingState mismatch")

			overall := CalculateOverallStatus(status.State, status.ProcessingState)
			Expect(overall).To(Equal(expectedOverall), "OverallStatus mismatch")
		},

		// ──────────────────────────────────────────────────────────────
		// 1. No conditions at all
		// ──────────────────────────────────────────────────────────────
		Entry("no conditions → Pending",
			[]conditionInput{},
			int64(0),
			api.None, api.ProcessingStatePending, api.OverallStatusPending,
		),

		// ──────────────────────────────────────────────────────────────
		// 2. Ready=True (success path) — reason is informational
		// ──────────────────────────────────────────────────────────────
		Entry("Ready=True/Provisioned + Processing=Done → Complete",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "", 0},
				{typeReady, metav1.ConditionTrue, condition.ReasonProvisioned, "", 0},
			},
			int64(0),
			api.Complete, api.ProcessingStateDone, api.OverallStatusComplete,
		),
		Entry("Ready=True wins even when Processing=Blocked",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "zone not ready", 0},
				{typeReady, metav1.ConditionTrue, condition.ReasonProvisioned, "", 0},
			},
			int64(0),
			api.Complete, api.ProcessingStateDone, api.OverallStatusComplete,
		),
		Entry("Ready=True wins even when Processing=True (still reconciling)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionTrue, condition.ReasonProcessing, "", 0},
				{typeReady, metav1.ConditionTrue, condition.ReasonProvisioned, "", 0},
			},
			int64(0),
			api.Complete, api.ProcessingStateDone, api.OverallStatusComplete,
		),

		// ──────────────────────────────────────────────────────────────
		// 3. Ready=False with processing-equivalent reasons
		//    These indicate transient progress — will resolve on their own.
		// ──────────────────────────────────────────────────────────────
		Entry("Ready=False/SubResourceNotReady → Processing",
			[]conditionInput{
				{typeProcessing, metav1.ConditionTrue, condition.ReasonProcessing, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonSubResourceNotReady, "child not ready", 0},
			},
			int64(0),
			api.None, api.ProcessingStateProcessing, api.OverallStatusProcessing,
		),
		Entry("Ready=False/Provisioning → Processing",
			[]conditionInput{
				{typeProcessing, metav1.ConditionTrue, condition.ReasonProvisioning, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonProvisioning, "setting up resources", 0},
			},
			int64(0),
			api.None, api.ProcessingStateProcessing, api.OverallStatusProcessing,
		),
		Entry("Ready=False/Processing (legacy) → Processing",
			[]conditionInput{
				{typeProcessing, metav1.ConditionTrue, condition.ReasonProcessing, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonProcessing, "reconciling", 0},
			},
			int64(0),
			api.None, api.ProcessingStateProcessing, api.OverallStatusProcessing,
		),

		// ──────────────────────────────────────────────────────────────
		// 4. Ready=False with blocked reasons
		//    Require external intervention to make progress.
		// ──────────────────────────────────────────────────────────────
		Entry("Ready=False/PreconditionNotMet → Blocked",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonPreconditionNotMet, "API not found", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=False/ApprovalPending → Blocked",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonApprovalPending, "waiting for approval", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=False/AccessDenied → Blocked",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonAccessDenied, "approval denied", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=False/ValidationFailed → Blocked",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonValidationFailed, "OAS lint errors", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=False/Blocked (generic) → Blocked",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonBlocked, "blocked by dependency", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),

		// ──────────────────────────────────────────────────────────────
		// 5. Ready=False with error reasons
		//    Internal errors — not user-controllable.
		// ──────────────────────────────────────────────────────────────
		Entry("Ready=False/Error → Invalid/Failed",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "", 0},
				{typeReady, metav1.ConditionFalse, condition.ReasonError, "internal controller error", 0},
			},
			int64(0),
			api.Invalid, api.ProcessingStateFailed, api.OverallStatusFailed,
		),

		// ──────────────────────────────────────────────────────────────
		// 6. Ready=False with unknown/unexpected reasons
		//    Defensive default: treat as Blocked/Done.
		// ──────────────────────────────────────────────────────────────
		Entry("Ready=False/UnknownReason (no Processing) → Blocked/Done",
			[]conditionInput{
				{typeReady, metav1.ConditionFalse, "SomeFutureReason", "something happened", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=False/UnknownReason + Processing=True → Processing (tiebreaker)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionTrue, condition.ReasonProcessing, "still going", 0},
				{typeReady, metav1.ConditionFalse, "SomeFutureReason", "something happened", 0},
			},
			int64(0),
			api.None, api.ProcessingStateProcessing, api.OverallStatusProcessing,
		),
		Entry("Ready=False/UnknownReason + Processing=Blocked → Blocked/Done (tiebreaker)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "stuck", 0},
				{typeReady, metav1.ConditionFalse, "SomeFutureReason", "something", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=False/UnknownReason + Processing=Done → Blocked/Done (defensive)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "finished", 0},
				{typeReady, metav1.ConditionFalse, "SomeFutureReason", "not ideal", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),

		// ──────────────────────────────────────────────────────────────
		// 7. Ready=Unknown — abnormal, fallback to Processing condition
		// ──────────────────────────────────────────────────────────────
		Entry("Ready=Unknown + Processing=True → Processing (fallback)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionTrue, condition.ReasonProcessing, "", 0},
				{typeReady, metav1.ConditionUnknown, "Unknown", "", 0},
			},
			int64(0),
			api.None, api.ProcessingStateProcessing, api.OverallStatusProcessing,
		),
		Entry("Ready=Unknown + Processing=Blocked → Blocked (fallback)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "missing label", 0},
				{typeReady, metav1.ConditionUnknown, "Unknown", "", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=Unknown + Processing=Done → None/Done (fallback)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "", 0},
				{typeReady, metav1.ConditionUnknown, "Unknown", "", 0},
			},
			int64(0),
			api.None, api.ProcessingStateDone, api.OverallStatusNone,
		),

		// ──────────────────────────────────────────────────────────────
		// 8. Ready=nil — only Processing condition present
		// ──────────────────────────────────────────────────────────────
		Entry("Ready=nil + Processing=True → Processing",
			[]conditionInput{
				{typeProcessing, metav1.ConditionTrue, condition.ReasonProcessing, "", 0},
			},
			int64(0),
			api.None, api.ProcessingStateProcessing, api.OverallStatusProcessing,
		),
		Entry("Ready=nil + Processing=Blocked → Blocked",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonBlocked, "env not set up", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=nil + Processing=Done → None/Done",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "", 0},
			},
			int64(0),
			api.None, api.ProcessingStateDone, api.OverallStatusNone,
		),

		// ──────────────────────────────────────────────────────────────
		// 9. Staleness detection
		// ──────────────────────────────────────────────────────────────
		Entry("Ready is stale (ObservedGeneration < objectGeneration) → Pending",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "", 2},
				{typeReady, metav1.ConditionTrue, condition.ReasonProvisioned, "", 1},
			},
			int64(2),
			api.None, api.ProcessingStatePending, api.OverallStatusPending,
		),
		Entry("Ready is current (ObservedGeneration == objectGeneration) → Complete",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "", 2},
				{typeReady, metav1.ConditionTrue, condition.ReasonProvisioned, "", 2},
			},
			int64(2),
			api.Complete, api.ProcessingStateDone, api.OverallStatusComplete,
		),
		Entry("ObservedGeneration=0 skips staleness (backward compat)",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "", 0},
				{typeReady, metav1.ConditionTrue, condition.ReasonProvisioned, "", 0},
			},
			int64(5),
			api.Complete, api.ProcessingStateDone, api.OverallStatusComplete,
		),
		Entry("objectGeneration=0 skips staleness",
			[]conditionInput{
				{typeProcessing, metav1.ConditionFalse, condition.ReasonDone, "", 1},
				{typeReady, metav1.ConditionTrue, condition.ReasonProvisioned, "", 1},
			},
			int64(0),
			api.Complete, api.ProcessingStateDone, api.OverallStatusComplete,
		),

		// ──────────────────────────────────────────────────────────────
		// 10. Message propagation checks
		// ──────────────────────────────────────────────────────────────
		// (State/ProcessingState only — message content is verified below.)
		Entry("Ready=False/ApprovalPending message propagated → Blocked",
			[]conditionInput{
				{typeReady, metav1.ConditionFalse, condition.ReasonApprovalPending, "Approval required for scope X", 0},
			},
			int64(0),
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked,
		),
		Entry("Ready=False/Error message propagated → Invalid/Failed",
			[]conditionInput{
				{typeReady, metav1.ConditionFalse, condition.ReasonError, "gateway returned 500", 0},
			},
			int64(0),
			api.Invalid, api.ProcessingStateFailed, api.OverallStatusFailed,
		),
	)

	// Verify that blocked/error reasons propagate the Ready message into
	// Warnings or Errors respectively.
	DescribeTable("message propagation",
		func(
			reason string,
			message string,
			expectWarnings bool,
			expectErrors bool,
		) {
			conditions := []conditionInput{
				{typeReady, metav1.ConditionFalse, reason, message, 0},
			}
			status := &api.Status{}
			fillStateInfo(toConditions(conditions), 0, status)

			if expectWarnings {
				Expect(status.Warnings).NotTo(BeEmpty(), "expected Warnings to contain the message")
				Expect(status.Warnings[0].Message).To(Equal(message))
			}
			if expectErrors {
				Expect(status.Errors).NotTo(BeEmpty(), "expected Errors to contain the message")
				Expect(status.Errors[0].Message).To(Equal(message))
			}
		},
		Entry("PreconditionNotMet → Warnings",
			condition.ReasonPreconditionNotMet, "API not found", true, false,
		),
		Entry("ApprovalPending → Warnings",
			condition.ReasonApprovalPending, "Waiting for approval", true, false,
		),
		Entry("AccessDenied → Warnings",
			condition.ReasonAccessDenied, "Approval denied", true, false,
		),
		Entry("ValidationFailed → Warnings",
			condition.ReasonValidationFailed, "OAS lint errors", true, false,
		),
		Entry("Blocked → Warnings",
			condition.ReasonBlocked, "Blocked by dependency", true, false,
		),
		Entry("Error → Errors",
			condition.ReasonError, "Internal controller error", false, true,
		),
		Entry("Unknown reason → Warnings (defensive default)",
			"SomeFutureReason", "Something went wrong", true, false,
		),
	)
})

var _ = Describe("isStale", func() {
	DescribeTable("staleness detection",
		func(observedGen, objectGen int64, expected bool) {
			cond := &metav1.Condition{ObservedGeneration: observedGen}
			Expect(isStale(cond, objectGen)).To(Equal(expected))
		},
		Entry("behind → stale", int64(1), int64(2), true),
		Entry("current → not stale", int64(2), int64(2), false),
		Entry("ahead → not stale", int64(3), int64(2), false),
		Entry("observedGen=0 → not stale (backward compat)", int64(0), int64(5), false),
		Entry("objectGen=0 → not stale", int64(3), int64(0), false),
		Entry("both zero → not stale", int64(0), int64(0), false),
	)

	It("returns false for nil condition", func() {
		Expect(isStale(nil, 5)).To(BeFalse())
	})
})

var _ = Describe("reconcileWithSubResources", func() {
	DescribeTable("adjusts parent state based on sub-resource reality",
		func(
			parentReadyReason string,
			parentState api.State,
			parentPS api.ProcessingState,
			worstSubStatus api.OverallStatus,
			hasActive bool,
			expectedState api.State,
			expectedPS api.ProcessingState,
		) {
			conditions := toConditions([]conditionInput{
				{typeReady, metav1.ConditionFalse, parentReadyReason, "test message", 0},
			})
			status := &api.Status{
				State:           parentState,
				ProcessingState: parentPS,
			}
			result := ProblemsResult{
				WorstOverallStatus:    worstSubStatus,
				HasActiveSubResources: hasActive,
			}
			reconcileWithSubResources(conditions, status, result)
			Expect(status.State).To(Equal(expectedState))
			Expect(status.ProcessingState).To(Equal(expectedPS))
		},

		// --- Override fires: all subs stuck, parent waiting on sub-resources ---
		Entry("all subs blocked, none active → override to blocked",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusBlocked, false,
			api.Blocked, api.ProcessingStateDone,
		),
		Entry("all subs failed, none active → override to invalid/failed",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusFailed, false,
			api.Invalid, api.ProcessingStateFailed,
		),
		Entry("all subs invalid, none active → override to invalid/failed",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusInvalid, false,
			api.Invalid, api.ProcessingStateFailed,
		),

		// --- No override: at least one sub still active ---
		Entry("worst=blocked but one sub still processing → no override",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusBlocked, true,
			api.None, api.ProcessingStateProcessing,
		),
		Entry("worst=failed but one sub still processing → no override",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusFailed, true,
			api.None, api.ProcessingStateProcessing,
		),

		// --- No override: parent is processing for own reasons (not SubResourceNotReady) ---
		Entry("parent Provisioning + subs blocked → no override",
			condition.ReasonProvisioning, api.None, api.ProcessingStateProcessing,
			api.OverallStatusBlocked, false,
			api.None, api.ProcessingStateProcessing,
		),
		Entry("parent Processing + subs blocked → no override",
			"Processing", api.None, api.ProcessingStateProcessing,
			api.OverallStatusBlocked, false,
			api.None, api.ProcessingStateProcessing,
		),

		// --- No override: parent is not in processing state ---
		Entry("parent already done → no override",
			condition.ReasonSubResourceNotReady, api.Blocked, api.ProcessingStateDone,
			api.OverallStatusBlocked, false,
			api.Blocked, api.ProcessingStateDone,
		),
		Entry("parent pending → no override",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStatePending,
			api.OverallStatusBlocked, false,
			api.None, api.ProcessingStatePending,
		),

		// --- No override: sub-resource status not worse than processing ---
		Entry("worst=processing, one sub still active → no override",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusProcessing, true,
			api.None, api.ProcessingStateProcessing,
		),
		Entry("worst=complete → no override (switch doesn't match)",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusComplete, false,
			api.None, api.ProcessingStateProcessing,
		),

		// --- Mixed scenarios (the user's key question) ---
		Entry("sub A complete + sub B processing → no override (HasActive=true from B)",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusProcessing, true,
			api.None, api.ProcessingStateProcessing,
		),
		Entry("sub A complete + sub B blocked, none active → override to blocked",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusBlocked, false,
			api.Blocked, api.ProcessingStateDone,
		),
		Entry("sub A processing + sub B blocked → HasActive=true, no override",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusBlocked, true,
			api.None, api.ProcessingStateProcessing,
		),
		Entry("sub A pending + sub B blocked → HasActive=true, no override",
			condition.ReasonSubResourceNotReady, api.None, api.ProcessingStateProcessing,
			api.OverallStatusBlocked, true,
			api.None, api.ProcessingStateProcessing,
		),
	)
})
