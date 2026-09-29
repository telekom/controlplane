// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// refKey identifies a notification ref by the fields AppendUniqueRef compares.
type refKey struct {
	namespace string
	name      string
}

func uniqueRefCount(refs []ctypes.ObjectRef) int {
	seen := make(map[refKey]struct{}, len(refs))
	for _, r := range refs {
		seen[refKey{namespace: r.Namespace, name: r.Name}] = struct{}{}
	}
	return len(seen)
}

var _ = Describe("Bounded decisions and deduplicated notification refs", Ordered, func() {
	const resourceName = "bounded-decisions"

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}

	requester := approvalv1.Requester{
		TeamName:  "test--requester",
		TeamEmail: "requester@example.com",
		Reason:    "Need access for testing",
		ApplicationRef: &ctypes.TypedObjectRef{
			ObjectRef: ctypes.ObjectRef{Name: "requester-app-name", Namespace: testNamespace},
		},
	}

	decider := approvalv1.Decider{
		TeamName:  "test--decider",
		TeamEmail: "decider@example.com",
		ApplicationRef: &ctypes.TypedObjectRef{
			ObjectRef: ctypes.ObjectRef{Name: "decider-app-name", Namespace: testNamespace},
		},
	}

	target := ctypes.TypedObjectRef{
		TypeMeta:  metav1.TypeMeta{Kind: "Subscription"},
		ObjectRef: ctypes.ObjectRef{Name: resourceName, Namespace: testNamespace},
	}

	// cycle flips the approval between Granted and Suspended, recording one
	// decision per transition through AppendDecision.
	cycle := func(i int) {
		var nextState approvalv1.ApprovalState

		// Retry on conflict: the controller concurrently writes finalizer/status.
		Eventually(func(g Gomega) {
			approval := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, namespacedName, approval)).To(Succeed())

			nextState = approvalv1.ApprovalStateSuspended
			if approval.Spec.State == approvalv1.ApprovalStateSuspended {
				nextState = approvalv1.ApprovalStateGranted
			}

			approval.Spec.State = nextState
			approval.AppendDecision(approvalv1.Decision{
				Name:           fmt.Sprintf("decider-%d", i),
				Email:          fmt.Sprintf("decider-%d@example.com", i),
				Comment:        fmt.Sprintf("transition %d", i),
				ResultingState: nextState,
			})
			g.Expect(k8sClient.Update(ctx, approval)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
			g.Expect(updated.Status.LastState).To(Equal(nextState))
		}, timeout, interval).Should(Succeed())
	}

	BeforeAll(func() {
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: approvalv1.ApprovalSpec{
				Action:    "subscribe",
				Target:    target,
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategySimple,
				State:     approvalv1.ApprovalStateGranted,
				Decisions: []approvalv1.Decision{{
					Name:           "decider-0",
					Email:          "decider-0@example.com",
					Comment:        "initial grant",
					ResultingState: approvalv1.ApprovalStateGranted,
				}},
			},
		}
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
			g.Expect(updated.Status.LastState).To(Equal(approvalv1.ApprovalStateGranted))
		}, timeout, interval).Should(Succeed())
	})

	AfterAll(func() {
		approval := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, namespacedName, approval)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, approval)).To(Succeed())
	})

	It("keeps only the most recent MaxDecisions after repeated state changes", func() {
		// 7 transitions on top of the initial decision => 8 recorded decisions.
		for i := 1; i <= 7; i++ {
			cycle(i)
		}

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
			g.Expect(updated.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))
			g.Expect(updated.Spec.Decisions[approvalv1.MaxDecisions-1].Name).To(Equal("decider-7"))
			g.Expect(updated.Spec.Decisions[0].Name).To(Equal("decider-3"))
		}, timeout, interval).Should(Succeed())
	})

	It("records each notification ref at most once", func() {
		updated := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())

		refs := updated.Status.NotificationRefs
		Expect(refs).NotTo(BeEmpty())
		Expect(refs).To(HaveLen(uniqueRefCount(refs)))
	})
})

var _ = Describe("ApprovalRequest notification refs deduplication", Ordered, func() {
	const resourceName = "dedup-notification-refs"

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}

	BeforeAll(func() {
		approvalReq := &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: approvalv1.ApprovalRequestSpec{
				Action:   "subscribe",
				Strategy: approvalv1.ApprovalStrategyFourEyes,
				State:    approvalv1.ApprovalStatePending,
				Target: ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{Kind: "Subscription"},
					ObjectRef: ctypes.ObjectRef{Name: resourceName, Namespace: testNamespace},
				},
				Requester: approvalv1.Requester{
					TeamName:  "test--requester",
					TeamEmail: "requester@example.com",
					Reason:    "Need access for testing",
					ApplicationRef: &ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "requester-app-name", Namespace: testNamespace},
					},
				},
				Decider: approvalv1.Decider{
					TeamName:  "test--decider",
					TeamEmail: "decider@example.com",
					ApplicationRef: &ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "decider-app-name", Namespace: testNamespace},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, approvalReq)).To(Succeed())
	})

	AfterAll(func() {
		approvalReq := &approvalv1.ApprovalRequest{}
		err := k8sClient.Get(ctx, namespacedName, approvalReq)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, approvalReq)).To(Succeed())
	})

	It("does not duplicate refs across repeated state changes", func() {
		// After generation 1 every notification uses the "updated" scenario, so
		// repeated state changes produce the very same deterministic Notification
		// names for the same actor.
		transitions := []approvalv1.ApprovalState{
			approvalv1.ApprovalStateRejected,
			approvalv1.ApprovalStateSemigranted,
			approvalv1.ApprovalStateRejected,
		}

		for i, nextState := range transitions {
			// Retry on conflict: the controller concurrently writes finalizer/status.
			Eventually(func(g Gomega) {
				updated := &approvalv1.ApprovalRequest{}
				g.Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
				updated.Spec.State = nextState
				updated.AppendDecision(approvalv1.Decision{
					Name:           fmt.Sprintf("decider-%d", i),
					Email:          fmt.Sprintf("decider-%d@example.com", i),
					ResultingState: nextState,
				})
				g.Expect(k8sClient.Update(ctx, updated)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				current := &approvalv1.ApprovalRequest{}
				g.Expect(k8sClient.Get(ctx, namespacedName, current)).To(Succeed())
				g.Expect(current.Status.LastState).To(Equal(nextState))
			}, timeout, interval).Should(Succeed())
		}

		Consistently(func(g Gomega) {
			current := &approvalv1.ApprovalRequest{}
			g.Expect(k8sClient.Get(ctx, namespacedName, current)).To(Succeed())
			refs := current.Status.NotificationRefs
			g.Expect(refs).NotTo(BeEmpty())
			g.Expect(refs).To(HaveLen(uniqueRefCount(refs)))
		}, 2*time.Second, interval).Should(Succeed())
	})
})

var _ = Describe("Expiration with a full decision list", Ordered, func() {
	const resourceName = "bounded-decisions-expiry"

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}

	fullDecisions := func() []approvalv1.Decision {
		decisions := make([]approvalv1.Decision, 0, approvalv1.MaxDecisions)
		for i := 0; i < approvalv1.MaxDecisions; i++ {
			decisions = append(decisions, approvalv1.Decision{
				Name:           fmt.Sprintf("decider-%d", i),
				Email:          fmt.Sprintf("decider-%d@example.com", i),
				ResultingState: approvalv1.ApprovalStateGranted,
			})
		}
		return decisions
	}

	BeforeAll(func() {
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: approvalv1.ApprovalSpec{
				Action: "subscribe",
				Target: ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{Kind: "Subscription"},
					ObjectRef: ctypes.ObjectRef{Name: resourceName, Namespace: testNamespace},
				},
				Requester: approvalv1.Requester{
					TeamName:  "test--requester",
					TeamEmail: "requester@example.com",
					Reason:    "Need access for testing",
					ApplicationRef: &ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "requester-app-name", Namespace: testNamespace},
					},
				},
				Decider: approvalv1.Decider{
					TeamName:  "test--decider",
					TeamEmail: "decider@example.com",
					ApplicationRef: &ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "decider-app-name", Namespace: testNamespace},
					},
				},
				Strategy:  approvalv1.ApprovalStrategySimple,
				State:     approvalv1.ApprovalStateGranted,
				Decisions: fullDecisions(),
			},
		}
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())
	})

	AfterAll(func() {
		approval := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, namespacedName, approval)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, approval)).To(Succeed())
	})

	It("replaces the oldest decision with the System Expired decision", func() {
		By("waiting for the ApprovalExpiration to be created")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, namespacedName, &approvalv1.ApprovalExpiration{})).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("moving the expiration deadline into the past")
		Eventually(func(g Gomega) {
			expiration := &approvalv1.ApprovalExpiration{}
			g.Expect(k8sClient.Get(ctx, namespacedName, expiration)).To(Succeed())
			expiration.Spec.Expiration = metav1.NewTime(time.Now().Add(-time.Hour))
			g.Expect(k8sClient.Update(ctx, expiration)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, namespacedName, updated)).To(Succeed())
			g.Expect(updated.Spec.State).To(Equal(approvalv1.ApprovalStateExpired))
			g.Expect(updated.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))

			last := updated.Spec.Decisions[approvalv1.MaxDecisions-1]
			g.Expect(last.Name).To(Equal(approvalv1.SystemDecisionName))
			g.Expect(last.ResultingState).To(Equal(approvalv1.ApprovalStateExpired))

			// The oldest decision has been dropped to make room.
			g.Expect(updated.Spec.Decisions[0].Name).To(Equal("decider-1"))
		}, timeout, interval).Should(Succeed())
	})
})
