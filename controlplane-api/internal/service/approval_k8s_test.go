// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApprovalK8sService", func() {
	var svc service.ApprovalService

	approvalReq := &approvalv1.ApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ar-1",
			Namespace: "dev--team-alpha",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "poc",
			},
		},
		Spec: approvalv1.ApprovalRequestSpec{
			State:    approvalv1.ApprovalStatePending,
			Strategy: approvalv1.ApprovalStrategySimple,
			Decider:  approvalv1.Decider{TeamName: "team-beta"},
		},
		Status: approvalv1.ApprovalRequestStatus{
			AvailableTransitions: approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted},
				{Action: approvalv1.ApprovalActionDeny, To: approvalv1.ApprovalStateRejected},
			},
		},
	}

	approval := &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "appr-1",
			Namespace: "dev--team-alpha",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "poc",
			},
		},
		Spec: approvalv1.ApprovalSpec{
			State:    approvalv1.ApprovalStateGranted,
			Strategy: approvalv1.ApprovalStrategySimple,
			Decider:  approvalv1.Decider{TeamName: "team-beta"},
		},
		Status: approvalv1.ApprovalStatus{
			AvailableTransitions: approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionSuspend, To: approvalv1.ApprovalStateSuspended},
			},
		},
	}

	BeforeEach(func() {
		k8sClient := newFakeClient(approvalReq.DeepCopy(), approval.DeepCopy())
		svc = service.NewApprovalK8sService(cc.NewScopedClient(k8sClient, "poc"))
	})

	Describe("DecideApprovalRequest", func() {
		It("should allow deciding an approval request", func() {
			ref := service.ResourceRef{
				Namespace: "dev--team-alpha",
				Name:      "ar-1",
				TeamName:  "team-beta",
			}
			input := model.DecisionInput{
				Action:  model.ApprovalActionAllow,
				Comment: strPtr("Looks good"),
			}
			payload, err := svc.DecideApprovalRequest(adminCtx(), ref, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Accepted).To(BeTrue())
			Expect(payload.Errors).To(BeEmpty())
		})

		It("should stamp user identity from viewer onto the decision", func() {
			ctx := viewer.NewContext(context.Background(), &viewer.Viewer{
				Admin:     true,
				UserName:  "Jane Doe",
				UserEmail: "jane@example.com",
			})
			ref := service.ResourceRef{
				Namespace: "dev--team-alpha",
				Name:      "ar-1",
				TeamName:  "team-beta",
			}
			input := model.DecisionInput{
				Action:  model.ApprovalActionAllow,
				Comment: strPtr("LGTM"),
			}
			payload, err := svc.DecideApprovalRequest(ctx, ref, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Accepted).To(BeTrue())
			Expect(payload.Errors).To(BeEmpty())
		})

		It("should return error for unavailable transition", func() {
			ref := service.ResourceRef{
				Namespace: "dev--team-alpha",
				Name:      "ar-1",
				TeamName:  "team-beta",
			}
			input := model.DecisionInput{
				Action: model.ApprovalActionSuspend,
			}
			payload, err := svc.DecideApprovalRequest(adminCtx(), ref, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Errors).ToNot(BeEmpty())
			Expect(payload.Errors[0].Code).To(Equal(model.ErrorCodePreconditionFailed))
		})
	})

	Describe("DecideApproval", func() {
		It("should allow deciding an approval", func() {
			ref := service.ResourceRef{
				Namespace: "dev--team-alpha",
				Name:      "appr-1",
				TeamName:  "team-beta",
			}
			input := model.DecisionInput{
				Action:  model.ApprovalActionSuspend,
				Comment: strPtr("Suspending"),
			}
			payload, err := svc.DecideApproval(adminCtx(), ref, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Accepted).To(BeTrue())
			Expect(payload.Errors).To(BeEmpty())
		})
	})
})

var _ = Describe("ApprovalK8sService decision list bounds", func() {
	var svc service.ApprovalService
	var k8sClient client.Client

	const namespace = "dev--team-alpha"

	fullDecisions := func(state approvalv1.ApprovalState) []approvalv1.Decision {
		decisions := make([]approvalv1.Decision, 0, approvalv1.MaxDecisions)
		for i := 0; i < approvalv1.MaxDecisions; i++ {
			decisions = append(decisions, approvalv1.Decision{
				Name:           fmt.Sprintf("decider-%d", i),
				Email:          fmt.Sprintf("decider-%d@example.com", i),
				ResultingState: state,
			})
		}
		return decisions
	}

	BeforeEach(func() {
		fullAR := &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ar-full",
				Namespace: namespace,
				Labels:    map[string]string{config.EnvironmentLabelKey: "poc"},
			},
			Spec: approvalv1.ApprovalRequestSpec{
				State:     approvalv1.ApprovalStatePending,
				Strategy:  approvalv1.ApprovalStrategySimple,
				Decider:   approvalv1.Decider{TeamName: "team-beta"},
				Decisions: fullDecisions(approvalv1.ApprovalStatePending),
			},
			Status: approvalv1.ApprovalRequestStatus{
				AvailableTransitions: approvalv1.AvailableTransitions{
					{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted},
				},
			},
		}

		fullApproval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "appr-full",
				Namespace: namespace,
				Labels:    map[string]string{config.EnvironmentLabelKey: "poc"},
			},
			Spec: approvalv1.ApprovalSpec{
				State:     approvalv1.ApprovalStateGranted,
				Strategy:  approvalv1.ApprovalStrategySimple,
				Decider:   approvalv1.Decider{TeamName: "team-beta"},
				Decisions: fullDecisions(approvalv1.ApprovalStateGranted),
			},
			Status: approvalv1.ApprovalStatus{
				AvailableTransitions: approvalv1.AvailableTransitions{
					{Action: approvalv1.ApprovalActionSuspend, To: approvalv1.ApprovalStateSuspended},
				},
			},
		}

		k8sClient = newFakeClient(fullAR, fullApproval)
		svc = service.NewApprovalK8sService(cc.NewScopedClient(k8sClient, "poc"))
	})

	It("keeps an already full ApprovalRequest at MaxDecisions", func() {
		ref := service.ResourceRef{Namespace: namespace, Name: "ar-full", TeamName: "team-beta"}
		input := model.DecisionInput{Action: model.ApprovalActionAllow, Comment: strPtr("LGTM")}

		payload, err := svc.DecideApprovalRequest(adminCtx(), ref, input)
		Expect(err).ToNot(HaveOccurred())
		Expect(payload.Accepted).To(BeTrue())
		Expect(payload.Errors).To(BeEmpty())

		stored := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(adminCtx(), k8stypes.NamespacedName{Name: "ar-full", Namespace: namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))
		Expect(stored.Spec.Decisions[0].Name).To(Equal("decider-1"))
		Expect(stored.Spec.Decisions[approvalv1.MaxDecisions-1].Comment).To(Equal("LGTM"))
	})

	It("keeps an already full Approval at MaxDecisions", func() {
		ref := service.ResourceRef{Namespace: namespace, Name: "appr-full", TeamName: "team-beta"}
		input := model.DecisionInput{Action: model.ApprovalActionSuspend, Comment: strPtr("Suspending")}

		payload, err := svc.DecideApproval(adminCtx(), ref, input)
		Expect(err).ToNot(HaveOccurred())
		Expect(payload.Accepted).To(BeTrue())
		Expect(payload.Errors).To(BeEmpty())

		stored := &approvalv1.Approval{}
		Expect(k8sClient.Get(adminCtx(), k8stypes.NamespacedName{Name: "appr-full", Namespace: namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))
		Expect(stored.Spec.Decisions[0].Name).To(Equal("decider-1"))
		Expect(stored.Spec.Decisions[approvalv1.MaxDecisions-1].Comment).To(Equal("Suspending"))
	})
})
