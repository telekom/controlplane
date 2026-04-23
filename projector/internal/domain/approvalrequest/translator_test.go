// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest_test

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/approvalrequest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApprovalRequest Translator", func() {
	var t approvalrequest.Translator

	Describe("ShouldSkip", func() {
		It("should skip when target name is empty", func() {
			obj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Action: "subscribe",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: ""},
					},
				},
			}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeTrue())
			Expect(reason).To(ContainSubstring("target.name"))
		})

		It("should skip when action is empty", func() {
			obj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Action: "",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: "my-sub"},
					},
				},
			}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeTrue())
			Expect(reason).To(ContainSubstring("action"))
		})

		It("should skip when target kind is not ApiSubscription", func() {
			obj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Action: "subscribe",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "OtherKind"},
						ObjectRef: ctypes.ObjectRef{Name: "my-sub"},
					},
				},
			}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeTrue())
			Expect(reason).To(ContainSubstring("ApiSubscription"))
		})

		It("should not skip a valid ApprovalRequest CR", func() {
			obj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Action: "subscribe",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: "my-sub"},
					},
					Decider: approvalv1.Decider{TeamName: "some-team"},
				},
			}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			reason := "need access"
			obj := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apisubscription--my-sub--abc123",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
					},
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Action:   "subscribe",
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    approvalv1.ApprovalStateGranted,
					Target: ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{
							Namespace: "prod--platform--narvi",
							Name:      "my-sub",
						},
					},
					Requester: approvalv1.Requester{
						TeamName:  "narvi",
						TeamEmail: "narvi@example.com",
						Reason:    reason,
						ApplicationRef: &ctypes.TypedObjectRef{
							ObjectRef: ctypes.ObjectRef{Name: "consumer-app"},
						},
					},
					Decider: approvalv1.Decider{
						TeamName:  "provider-team",
						TeamEmail: "provider@example.com",
					},
					Decisions: []approvalv1.Decision{
						{Name: "Alice", Email: "alice@example.com", Comment: "approved"},
					},
				},
				Status: approvalv1.ApprovalRequestStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionTrue,
							Message: "approval request granted",
						},
					},
					AvailableTransitions: approvalv1.AvailableTransitions{
						{Action: approvalv1.ApprovalActionDeny, To: approvalv1.ApprovalStateRejected},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Meta.Namespace).To(Equal("prod--platform--narvi"))
			Expect(data.Meta.Name).To(Equal("apisubscription--my-sub--abc123"))
			Expect(data.Meta.Environment).To(Equal("prod"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("approval request granted"))
			Expect(data.State).To(Equal("GRANTED"))
			Expect(data.Action).To(Equal("subscribe"))
			Expect(data.Strategy).To(Equal("FOUR_EYES"))
			Expect(data.Requester.TeamName).To(Equal("narvi"))
			Expect(data.Requester.TeamEmail).To(Equal("narvi@example.com"))
			Expect(*data.Requester.Reason).To(Equal("need access"))
			Expect(*data.Requester.ApplicationName).To(Equal("consumer-app"))
			Expect(data.Decider.TeamName).To(Equal("provider-team"))
			Expect(*data.Decider.TeamEmail).To(Equal("provider@example.com"))
			Expect(data.Decisions).To(HaveLen(1))
			Expect(data.Decisions[0].Name).To(Equal("Alice"))
			Expect(*data.Decisions[0].Email).To(Equal("alice@example.com"))
			Expect(*data.Decisions[0].Comment).To(Equal("approved"))
			Expect(data.AvailableTransitions).To(HaveLen(1))
			Expect(data.AvailableTransitions[0].Action).To(Equal("Deny"))
			Expect(data.AvailableTransitions[0].ToState).To(Equal("Rejected"))
			Expect(data.SubscriptionNamespace).To(Equal("prod--platform--narvi"))
			Expect(data.SubscriptionName).To(Equal("my-sub"))
		})

		It("should fall back to own namespace when target namespace is empty", func() {
			obj := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apisubscription--my-sub--abc123",
					Namespace: "prod--platform--narvi",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Action:   "subscribe",
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStatePending,
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: "my-sub"},
					},
					Requester: approvalv1.Requester{TeamName: "narvi"},
					Decider:   approvalv1.Decider{TeamName: "provider"},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SubscriptionNamespace).To(Equal("prod--platform--narvi"))
		})

		It("should return empty decisions slice when no decisions", func() {
			obj := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
				Spec: approvalv1.ApprovalRequestSpec{
					Action:   "subscribe",
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStatePending,
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: "sub"},
					},
					Requester: approvalv1.Requester{TeamName: "t"},
					Decider:   approvalv1.Decider{TeamName: "d"},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Decisions).To(Equal([]model.Decision{}))
			Expect(data.AvailableTransitions).To(Equal([]model.AvailableTransition{}))
		})
	})

	Describe("Strategy mapping", func() {
		newObj := func(strategy approvalv1.ApprovalStrategy) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
				Spec: approvalv1.ApprovalRequestSpec{
					Action:   "subscribe",
					Strategy: strategy,
					State:    approvalv1.ApprovalStatePending,
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: "sub"},
					},
					Requester: approvalv1.Requester{TeamName: "t"},
					Decider:   approvalv1.Decider{TeamName: "d"},
				},
			}
		}

		It("should map Auto to AUTO", func() {
			data, err := t.Translate(context.Background(), newObj(approvalv1.ApprovalStrategyAuto))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Strategy).To(Equal("AUTO"))
		})

		It("should map Simple to SIMPLE", func() {
			data, err := t.Translate(context.Background(), newObj(approvalv1.ApprovalStrategySimple))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Strategy).To(Equal("SIMPLE"))
		})

		It("should map FourEyes to FOUR_EYES", func() {
			data, err := t.Translate(context.Background(), newObj(approvalv1.ApprovalStrategyFourEyes))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Strategy).To(Equal("FOUR_EYES"))
		})
	})

	Describe("State mapping", func() {
		newObj := func(state approvalv1.ApprovalState) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
				Spec: approvalv1.ApprovalRequestSpec{
					Action:   "subscribe",
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    state,
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: "sub"},
					},
					Requester: approvalv1.Requester{TeamName: "t"},
					Decider:   approvalv1.Decider{TeamName: "d"},
				},
			}
		}

		It("should map Pending to PENDING", func() {
			data, err := t.Translate(context.Background(), newObj(approvalv1.ApprovalStatePending))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.State).To(Equal("PENDING"))
		})

		It("should map Granted to GRANTED", func() {
			data, err := t.Translate(context.Background(), newObj(approvalv1.ApprovalStateGranted))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.State).To(Equal("GRANTED"))
		})

		It("should map Rejected to REJECTED", func() {
			data, err := t.Translate(context.Background(), newObj(approvalv1.ApprovalStateRejected))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.State).To(Equal("REJECTED"))
		})

		It("should map Semigranted to SEMIGRANTED", func() {
			data, err := t.Translate(context.Background(), newObj(approvalv1.ApprovalStateSemigranted))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.State).To(Equal("SEMIGRANTED"))
		})
	})

	Describe("KeyFromObject", func() {
		It("should derive all key fields from the live object", func() {
			obj := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apisubscription--my-sub--abc123",
					Namespace: "prod--platform--narvi",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Action: "subscribe",
					Target: ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{
							Namespace: "prod--platform--narvi",
							Name:      "my-sub",
						},
					},
				},
			}

			key := t.KeyFromObject(obj)
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("apisubscription--my-sub--abc123"))
			Expect(key.SubscriptionNamespace).To(Equal("prod--platform--narvi"))
			Expect(key.SubscriptionName).To(Equal("my-sub"))
		})

		It("should fall back to own namespace when target namespace is empty", func() {
			obj := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apisubscription--my-sub--abc123",
					Namespace: "prod--platform--narvi",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Action: "subscribe",
					Target: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{Name: "my-sub"},
					},
				},
			}

			key := t.KeyFromObject(obj)
			Expect(key.SubscriptionNamespace).To(Equal("prod--platform--narvi"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should derive from lastKnown when available", func() {
			lastKnown := &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apisubscription--my-sub--abc123",
					Namespace: "prod--platform--narvi",
				},
				Spec: approvalv1.ApprovalRequestSpec{
					Action: "subscribe",
					Target: ctypes.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{Kind: "ApiSubscription"},
						ObjectRef: ctypes.ObjectRef{
							Namespace: "prod--platform--narvi",
							Name:      "my-sub",
						},
					},
				},
			}

			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "apisubscription--my-sub--abc123"}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("apisubscription--my-sub--abc123"))
			Expect(key.SubscriptionNamespace).To(Equal("prod--platform--narvi"))
			Expect(key.SubscriptionName).To(Equal("my-sub"))
		})

		It("should use namespace+name fallback when lastKnown is nil", func() {
			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "apisubscription--my-sub--abc123"}
			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("apisubscription--my-sub--abc123"))
			Expect(key.SubscriptionNamespace).To(BeEmpty())
			Expect(key.SubscriptionName).To(BeEmpty())
		})
	})
})
