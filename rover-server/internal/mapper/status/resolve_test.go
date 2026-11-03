// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("ResolveRoverStatus", func() {
	conditions := func(ready metav1.ConditionStatus, reason string) []metav1.Condition {
		processing := metav1.Condition{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: condition.ReasonDone, ObservedGeneration: 1}
		if reason == condition.ReasonSubResourceNotReady {
			processing.Status, processing.Reason = metav1.ConditionTrue, condition.ReasonSubResourceNotReady
		}
		return []metav1.Condition{
			processing,
			{Type: condition.ConditionTypeReady, Status: ready, Reason: reason, ObservedGeneration: 1},
		}
	}
	ready := conditions(metav1.ConditionTrue, condition.ReasonProvisioned)
	waiting := conditions(metav1.ConditionFalse, condition.ReasonSubResourceNotReady)
	pending := conditions(metav1.ConditionFalse, condition.ReasonApprovalPending)

	child := func(conditions []metav1.Condition, generation int64) *apiv1.ApiSubscription {
		return &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: "ns", Generation: generation},
			Status:     apiv1.ApiSubscriptionStatus{Conditions: conditions},
		}
	}

	DescribeTable("folds the sub-resources added to the result",
		func(roverConditions []metav1.Condition, children []*apiv1.ApiSubscription, expectedState api.State, expectedPS api.ProcessingState, expectedOverall api.OverallStatus) {
			rover := &v1.Rover{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Status: v1.RoverStatus{Conditions: roverConditions}}
			result := ProblemsResult{}
			for _, c := range children {
				result.Add(c)
			}

			status, overall := ResolveRoverStatus(rover, result)

			Expect(status.State).To(Equal(expectedState))
			Expect(status.ProcessingState).To(Equal(expectedPS))
			Expect(overall).To(Equal(expectedOverall))
		},
		Entry("a blocked sub-resource blocks a Rover waiting on it",
			waiting, []*apiv1.ApiSubscription{child(pending, 1)},
			api.Blocked, api.ProcessingStateDone, api.OverallStatusBlocked),
		Entry("a sub-resource still processing keeps the Rover processing next to a blocked one",
			waiting, []*apiv1.ApiSubscription{child(pending, 1), child(waiting, 1)},
			api.None, api.ProcessingStateProcessing, api.OverallStatusProcessing),
		Entry("a stale sub-resource keeps a complete Rover processing",
			ready, []*apiv1.ApiSubscription{child(ready, 2)},
			api.Complete, api.ProcessingStateProcessing, api.OverallStatusProcessing),
	)
})
