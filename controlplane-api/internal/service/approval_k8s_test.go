// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ApprovalK8sService", func() {
	var (
		svc       service.ApprovalService
		k8sClient client.Client
	)

	seedApprovalRequest := &approvalv1.ApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-sub--abc123",
			Namespace: "dev--requester-team",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "dev",
			},
		},
		Spec: approvalv1.ApprovalRequestSpec{
			Action: "subscribe",
			Requester: approvalv1.Requester{
				TeamName:  "requester-team",
				TeamEmail: "requester@example.com",
			},
			Decider: approvalv1.Decider{
				TeamName:  "decider-team",
				TeamEmail: "decider@example.com",
			},
			Strategy: approvalv1.ApprovalStrategySimple,
			State:    approvalv1.ApprovalStatePending,
		},
		Status: approvalv1.ApprovalRequestStatus{
			AvailableTransitions: approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted},
				{Action: approvalv1.ApprovalActionDeny, To: approvalv1.ApprovalStateRejected},
			},
		},
	}

	seedApproval := &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apisubscription--my-sub",
			Namespace: "dev--requester-team",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "dev",
			},
		},
		Spec: approvalv1.ApprovalSpec{
			Action: "subscribe",
			Requester: approvalv1.Requester{
				TeamName:  "requester-team",
				TeamEmail: "requester@example.com",
			},
			Decider: approvalv1.Decider{
				TeamName:  "decider-team",
				TeamEmail: "decider@example.com",
			},
			Strategy: approvalv1.ApprovalStrategySimple,
			State:    approvalv1.ApprovalStateGranted,
		},
		Status: approvalv1.ApprovalStatus{
			AvailableTransitions: approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionDeny, To: approvalv1.ApprovalStateRejected},
				{Action: approvalv1.ApprovalActionSuspend, To: approvalv1.ApprovalStateSuspended},
			},
		},
	}

	decisionInput := model.DecisionInput{
		Name:    "Alice Decider",
		Email:   strPtr("alice@decider.com"),
		Comment: strPtr("Looks good"),
	}

	Describe("DecideApprovalRequest", func() {
		decideInput := model.DecideApprovalRequestInput{
			Environment: "dev",
			Team:        "requester-team",
			Name:        "my-sub--abc123",
			Action:      "ALLOW",
			Decision:    decisionInput,
		}

		BeforeEach(func() {
			k8sClient = newFakeClient(seedApprovalRequest.DeepCopy())
			svc = service.NewApprovalK8sService(k8sClient)
		})

		Describe("Authorization", func() {
			It("should allow admin to decide on any approval request", func() {
				result, err := svc.DecideApprovalRequest(adminCtx(), decideInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should allow decider team to decide", func() {
				result, err := svc.DecideApprovalRequest(teamCtx("decider-team"), decideInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny non-decider team", func() {
				_, err := svc.DecideApprovalRequest(teamCtx("other-team"), decideInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should deny when no viewer is present", func() {
				_, err := svc.DecideApprovalRequest(noViewerCtx(), decideInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unauthorized"))
			})
		})

		Describe("Success", func() {
			It("should update state and append decision", func() {
				result, err := svc.DecideApprovalRequest(adminCtx(), decideInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
				Expect(result.Message).To(Equal("approval request decision applied"))
				Expect(*result.NewState).To(Equal("Granted"))
				Expect(*result.Namespace).To(Equal("dev--requester-team"))
				Expect(*result.ResourceName).To(Equal("my-sub--abc123"))

				// Verify the CRD was updated
				ar := &approvalv1.ApprovalRequest{}
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: "dev--requester-team",
					Name:      "my-sub--abc123",
				}, ar)
				Expect(err).NotTo(HaveOccurred())
				Expect(ar.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				Expect(ar.Spec.Decisions).To(HaveLen(1))
				Expect(ar.Spec.Decisions[0].Name).To(Equal("Alice Decider"))
				Expect(ar.Spec.Decisions[0].Email).To(Equal("alice@decider.com"))
				Expect(ar.Spec.Decisions[0].Comment).To(Equal("Looks good"))
				Expect(ar.Spec.Decisions[0].ResultingState).To(Equal(approvalv1.ApprovalStateGranted))
				Expect(ar.Spec.Decisions[0].Timestamp).NotTo(BeNil())
			})

			It("should deny an approval request", func() {
				denyInput := decideInput
				denyInput.Action = "DENY"
				result, err := svc.DecideApprovalRequest(adminCtx(), denyInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
				Expect(*result.NewState).To(Equal("Rejected"))
			})
		})

		Describe("Unavailable action", func() {
			It("should return error for action not in available transitions", func() {
				suspendInput := decideInput
				suspendInput.Action = "SUSPEND"
				_, err := svc.DecideApprovalRequest(adminCtx(), suspendInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not available"))
			})
		})

		Describe("Not found", func() {
			It("should return error when approval request does not exist", func() {
				_, err := svc.DecideApprovalRequest(adminCtx(), model.DecideApprovalRequestInput{
					Environment: "dev",
					Team:        "requester-team",
					Name:        "nonexistent",
					Action:      "ALLOW",
					Decision:    decisionInput,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})
	})

	Describe("DecideApproval", func() {
		decideInput := model.DecideApprovalInput{
			Environment: "dev",
			Team:        "requester-team",
			Name:        "apisubscription--my-sub",
			Action:      "SUSPEND",
			Decision:    decisionInput,
		}

		BeforeEach(func() {
			k8sClient = newFakeClient(seedApproval.DeepCopy())
			svc = service.NewApprovalK8sService(k8sClient)
		})

		Describe("Authorization", func() {
			It("should allow admin to decide on any approval", func() {
				result, err := svc.DecideApproval(adminCtx(), decideInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should allow decider team to decide", func() {
				result, err := svc.DecideApproval(teamCtx("decider-team"), decideInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny non-decider team", func() {
				_, err := svc.DecideApproval(teamCtx("other-team"), decideInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should deny when no viewer is present", func() {
				_, err := svc.DecideApproval(noViewerCtx(), decideInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unauthorized"))
			})
		})

		Describe("Success", func() {
			It("should suspend an approval and append decision", func() {
				result, err := svc.DecideApproval(adminCtx(), decideInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
				Expect(result.Message).To(Equal("approval decision applied"))
				Expect(*result.NewState).To(Equal("Suspended"))
				Expect(*result.Namespace).To(Equal("dev--requester-team"))
				Expect(*result.ResourceName).To(Equal("apisubscription--my-sub"))

				// Verify the CRD was updated
				approval := &approvalv1.Approval{}
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: "dev--requester-team",
					Name:      "apisubscription--my-sub",
				}, approval)
				Expect(err).NotTo(HaveOccurred())
				Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateSuspended))
				Expect(approval.Spec.Decisions).To(HaveLen(1))
				Expect(approval.Spec.Decisions[0].Name).To(Equal("Alice Decider"))
				Expect(approval.Spec.Decisions[0].ResultingState).To(Equal(approvalv1.ApprovalStateSuspended))
			})

			It("should deny an approval", func() {
				denyInput := decideInput
				denyInput.Action = "DENY"
				result, err := svc.DecideApproval(adminCtx(), denyInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
				Expect(*result.NewState).To(Equal("Rejected"))
			})
		})

		Describe("Unavailable action", func() {
			It("should return error for action not in available transitions", func() {
				resumeInput := decideInput
				resumeInput.Action = "RESUME"
				_, err := svc.DecideApproval(adminCtx(), resumeInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not available"))
			})
		})

		Describe("Not found", func() {
			It("should return error when approval does not exist", func() {
				_, err := svc.DecideApproval(adminCtx(), model.DecideApprovalInput{
					Environment: "dev",
					Team:        "requester-team",
					Name:        "nonexistent",
					Action:      "SUSPEND",
					Decision:    decisionInput,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})
	})
})
