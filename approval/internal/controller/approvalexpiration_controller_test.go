// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	commonclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApprovalExpiration Controller", Ordered, func() {
	const resourceName = "approval-expiration-lifecycle"

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}
	approvalExpirationName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}

	requester := approvalv1.Requester{
		TeamName:  "test--requester",
		TeamEmail: "requester@example.com",
		Reason:    "Need access for testing",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "requester-app-name",
				Namespace: testNamespace,
			},
		},
	}

	decider := approvalv1.Decider{
		TeamName:  "test--decider",
		TeamEmail: "decider@example.com",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "decider-app-name",
				Namespace: testNamespace,
			},
		},
	}

	target := ctypes.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{Kind: "Subscription"},
		ObjectRef: ctypes.ObjectRef{
			Name:      resourceName,
			Namespace: testNamespace,
		},
	}

	var firstExpiration metav1.Time
	var recreatedExpiration metav1.Time

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
					Name:           "Alice Approver",
					Email:          "alice@example.com",
					Comment:        "Initial approval",
					ResultingState: approvalv1.ApprovalStateGranted,
				}},
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

		Eventually(func(g Gomega) {
			getErr := k8sClient.Get(ctx, namespacedName, &approvalv1.Approval{})
			g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		expiration := &approvalv1.ApprovalExpiration{}
		err = k8sClient.Get(ctx, approvalExpirationName, expiration)
		if err == nil {
			Expect(k8sClient.Delete(ctx, expiration)).To(Succeed())
		} else {
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}
	})

	It("creates an ApprovalExpiration for a granted approval", func() {
		Eventually(func(g Gomega) {
			approval := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, approval)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			g.Expect(approval.Status.ExpiresAt).NotTo(BeNil())

			expiration := &approvalv1.ApprovalExpiration{}
			err = k8sClient.Get(ctx, approvalExpirationName, expiration)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(expiration.Spec.Approval.Name).To(Equal(resourceName))
			g.Expect(expiration.Spec.Approval.Namespace).To(Equal(testNamespace))
			g.Expect(approval.Status.ExpiresAt.Time).To(Equal(expiration.Spec.Expiration.Time))

			firstExpiration = expiration.Spec.Expiration
		}, timeout, interval).Should(Succeed())
	})

	It("deletes ApprovalExpiration and clears ExpiresAt when approval is rejected", func() {
		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, namespacedName, approval)).To(Succeed())
		approval.Spec.State = approvalv1.ApprovalStateRejected
		approval.Spec.Decisions = []approvalv1.Decision{{
			Name:           "Bob Decider",
			Email:          "bob@example.com",
			Comment:        "Rejecting access",
			ResultingState: approvalv1.ApprovalStateRejected,
		}}
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
			g.Expect(updated.Status.ExpiresAt).To(BeNil())
		}, timeout, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, approvalExpirationName, &approvalv1.ApprovalExpiration{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("recreates ApprovalExpiration with a new expiration when approval is granted again", func() {
		By("waiting long enough for the recreated expiration timestamp to move forward")
		time.Sleep(1100 * time.Millisecond)

		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, namespacedName, approval)).To(Succeed())
		approval.Spec.State = approvalv1.ApprovalStateGranted
		approval.Spec.Decisions = []approvalv1.Decision{{
			Name:           "Carol Decider",
			Email:          "carol@example.com",
			Comment:        "Granting access again",
			ResultingState: approvalv1.ApprovalStateGranted,
		}}
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			g.Expect(updated.Status.ExpiresAt).NotTo(BeNil())

			expiration := &approvalv1.ApprovalExpiration{}
			err = k8sClient.Get(ctx, approvalExpirationName, expiration)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.ExpiresAt.Time).To(Equal(expiration.Spec.Expiration.Time))
			g.Expect(expiration.Spec.Expiration.Time.After(firstExpiration.Time)).To(BeTrue())

			recreatedExpiration = expiration.Spec.Expiration
		}, timeout, interval).Should(Succeed())
	})

	It("keeps the same expiration when approval is suspended", func() {
		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, namespacedName, approval)).To(Succeed())
		approval.Spec.State = approvalv1.ApprovalStateSuspended
		approval.Spec.Decisions = []approvalv1.Decision{{
			Name:           "Dana Decider",
			Email:          "dana@example.com",
			Comment:        "Suspending access",
			ResultingState: approvalv1.ApprovalStateSuspended,
		}}
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Spec.State).To(Equal(approvalv1.ApprovalStateSuspended))
			g.Expect(updated.Status.ExpiresAt).NotTo(BeNil())
			g.Expect(updated.Status.ExpiresAt.Time).To(Equal(recreatedExpiration.Time))

			expiration := &approvalv1.ApprovalExpiration{}
			err = k8sClient.Get(ctx, approvalExpirationName, expiration)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(expiration.Spec.Expiration.Time).To(Equal(recreatedExpiration.Time))
		}, timeout, interval).Should(Succeed())
	})
})

var _ = Describe("ApprovalBuilder on an expired Approval", func() {
	requester := &approvalv1.Requester{
		TeamName:  "expiry--requester",
		TeamEmail: "requester@example.com",
		Reason:    "Need access for testing",
		ApplicationRef: &ctypes.TypedObjectRef{
			ObjectRef: ctypes.ObjectRef{Name: "requester-app-name", Namespace: testNamespace},
		},
	}
	decider := &approvalv1.Decider{
		TeamName:  "expiry--decider",
		TeamEmail: "decider@example.com",
		ApplicationRef: &ctypes.TypedObjectRef{
			ObjectRef: ctypes.ObjectRef{Name: "decider-app-name", Namespace: testNamespace},
		},
	}

	newOwner := func(name string) *test.TestResource {
		owner := test.NewObject(name, testNamespace)
		owner.SetUID(types.UID("uid-" + name))
		owner.SetLabels(map[string]string{config.EnvironmentLabelKey: testEnvironment})
		return owner
	}

	hashValue := func(owner *test.TestResource) any {
		return map[string]any{"owner": owner.GetName()}
	}

	approvalKey := func(owner *test.TestResource) types.NamespacedName {
		return types.NamespacedName{Name: approvalv1.ApprovalName("TestResource", owner.GetName()), Namespace: testNamespace}
	}

	// build runs the builder the way a domain controller does on each reconcile.
	build := func(owner *test.TestResource) builder.ApprovalResult {
		jclient := commonclient.NewJanitorClient(commonclient.NewScopedClient(mgrClient, testEnvironment))
		b := builder.NewApprovalBuilder(jclient, owner)
		b.WithHashValue(hashValue(owner))
		b.WithRequester(requester)
		b.WithDecider(decider)
		b.WithStrategy(approvalv1.ApprovalStrategyAuto)
		res, err := b.Build(ctx)
		Expect(err).NotTo(HaveOccurred())
		return res
	}

	// awaitGranted waits until the approval controller has reconciled the
	// granted Approval and created its ApprovalExpiration.
	awaitGranted := func(key types.NamespacedName) {
		Eventually(func(g Gomega) {
			approval := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, key, approval)).To(Succeed())
			g.Expect(approval.Status.LastState).To(Equal(approvalv1.ApprovalStateGranted))
			g.Expect(k8sClient.Get(ctx, key, &approvalv1.ApprovalExpiration{})).To(Succeed())
		}, timeout, interval).Should(Succeed())
	}

	// grant lets the builder request an Auto approval and waits until the
	// controllers have issued and reconciled the granted Approval.
	grant := func(owner *test.TestResource) types.NamespacedName {
		Expect(build(owner)).To(Equal(builder.ApprovalResultPending))
		key := approvalKey(owner)
		awaitGranted(key)
		return key
	}

	// grantWithoutDecisions creates a granted Approval without decisions, as
	// Auto approvals were created before the system recorded its decisions.
	grantWithoutDecisions := func(owner *test.TestResource) types.NamespacedName {
		key := approvalKey(owner)
		target := *ctypes.TypedObjectRefFromObject(owner, k8sClient.Scheme())
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
				Labels:    map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: approvalv1.ApprovalSpec{
				Action:    "subscribe",
				Target:    target,
				Requester: *requester,
				Decider:   *decider,
				Strategy:  approvalv1.ApprovalStrategyAuto,
				State:     approvalv1.ApprovalStateGranted,
				ApprovedRequest: &ctypes.ObjectRef{
					Name:      approvalv1.ApprovalRequestName(owner, hashValue(owner)),
					Namespace: testNamespace,
				},
			},
		}
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())
		awaitGranted(key)
		return key
	}

	// suspend sets the Approval to Suspended with the given decisions and
	// waits until the approval controller has reconciled the suspension.
	suspend := func(key types.NamespacedName, decisions func([]approvalv1.Decision) []approvalv1.Decision) {
		Eventually(func(g Gomega) {
			approval := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, key, approval)).To(Succeed())
			approval.Spec.State = approvalv1.ApprovalStateSuspended
			approval.Spec.Decisions = decisions(approval.Spec.Decisions)
			g.Expect(k8sClient.Update(ctx, approval)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			approval := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, key, approval)).To(Succeed())
			g.Expect(approval.Status.LastState).To(Equal(approvalv1.ApprovalStateSuspended))
		}, timeout, interval).Should(Succeed())
	}

	// expire moves the expiration into the past and waits until the approval
	// controller has reconciled the expiry, so Status.LastState is Expired.
	// It waits on the cached client the builder reads from and returns the
	// expired Approval.
	expire := func(key types.NamespacedName) *approvalv1.Approval {
		Eventually(func(g Gomega) {
			expiration := &approvalv1.ApprovalExpiration{}
			g.Expect(k8sClient.Get(ctx, key, expiration)).To(Succeed())
			expiration.Spec.Expiration = metav1.NewTime(time.Now().Add(-time.Minute))
			g.Expect(k8sClient.Update(ctx, expiration)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		approval := &approvalv1.Approval{}
		Eventually(func(g Gomega) {
			g.Expect(mgrClient.Get(ctx, key, approval)).To(Succeed())
			g.Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateExpired))
			g.Expect(approval.Status.LastState).To(Equal(approvalv1.ApprovalStateExpired))
		}, timeout, interval).Should(Succeed())
		return approval
	}

	resultingStates := func(approval *approvalv1.Approval) []approvalv1.ApprovalState {
		states := make([]approvalv1.ApprovalState, 0, len(approval.Spec.Decisions))
		for _, d := range approval.Spec.Decisions {
			states = append(states, d.ResultingState)
		}
		return states
	}

	expectApprovalGranted := func(owner *test.TestResource, status metav1.ConditionStatus) {
		cond := meta.FindStatusCondition(owner.GetConditions(), builder.ConditionTypeApprovalGranted)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(status))
	}

	It("keeps access granted when the Approval expired after a grant", func() {
		owner := newOwner("expired-after-grant")
		expire(grant(owner))

		owner = newOwner("expired-after-grant")
		Expect(build(owner)).To(Equal(builder.ApprovalResultGranted))
		expectApprovalGranted(owner, metav1.ConditionTrue)
	})

	It("denies access when the Approval expired after a suspension", func() {
		owner := newOwner("expired-after-suspension")
		key := grant(owner)
		suspend(key, func(ds []approvalv1.Decision) []approvalv1.Decision {
			return append(ds, approvalv1.Decision{Name: "Dana Decider", ResultingState: approvalv1.ApprovalStateSuspended})
		})
		expire(key)

		owner = newOwner("expired-after-suspension")
		Expect(build(owner)).To(Equal(builder.ApprovalResultDenied))
		expectApprovalGranted(owner, metav1.ConditionFalse)
	})

	It("denies access when the Approval expired after a suspension without a decision", func() {
		owner := newOwner("expired-after-silent-suspension")
		key := grant(owner)
		// A full history that ends with a grant, then a suspension edited
		// without a decision (for example by hand).
		suspend(key, func([]approvalv1.Decision) []approvalv1.Decision {
			states := []approvalv1.ApprovalState{
				approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateGranted,
				approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateGranted,
			}
			ds := make([]approvalv1.Decision, 0, len(states))
			for _, state := range states {
				ds = append(ds, approvalv1.Decision{Name: "Dana Decider", ResultingState: state})
			}
			return ds
		})
		approval := expire(key)

		owner = newOwner("expired-after-silent-suspension")
		Expect(build(owner)).To(Equal(builder.ApprovalResultDenied))
		expectApprovalGranted(owner, metav1.ConditionFalse)

		By("recording the suspension before the expiry, within the newest decisions")
		Expect(resultingStates(approval)).To(Equal([]approvalv1.ApprovalState{
			approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateGranted,
			approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateExpired,
		}))
		Expect(approval.Spec.Decisions[3].Name).To(Equal(approvalv1.SystemDecisionName))
	})

	It("keeps access granted when an Approval without decisions expired", func() {
		owner := newOwner("expired-without-decisions")
		approval := expire(grantWithoutDecisions(owner))

		owner = newOwner("expired-without-decisions")
		Expect(build(owner)).To(Equal(builder.ApprovalResultGranted))
		expectApprovalGranted(owner, metav1.ConditionTrue)

		By("recording the grant before the expiry")
		Expect(resultingStates(approval)).To(Equal([]approvalv1.ApprovalState{
			approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateExpired,
		}))
	})
})
