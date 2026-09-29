// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The CRD declares status.notificationRefs as a map-typed list keyed on
// namespace and name, so the API server itself rejects duplicates.
var _ = Describe("notificationRefs list-map schema", Ordered, func() {
	duplicateRefs := []ctypes.ObjectRef{
		{Namespace: testNamespace, Name: "notification-a"},
		{Namespace: testNamespace, Name: "notification-a"},
	}

	newRequester := func() approvalv1.Requester {
		return approvalv1.Requester{
			TeamName:  "test--requester",
			TeamEmail: "requester@example.com",
			Reason:    "Need access for testing",
			ApplicationRef: &ctypes.TypedObjectRef{
				ObjectRef: ctypes.ObjectRef{Name: "requester-app-name", Namespace: testNamespace},
			},
		}
	}

	newDecider := func() approvalv1.Decider {
		return approvalv1.Decider{
			TeamName:  "test--decider",
			TeamEmail: "decider@example.com",
			ApplicationRef: &ctypes.TypedObjectRef{
				ObjectRef: ctypes.ObjectRef{Name: "decider-app-name", Namespace: testNamespace},
			},
		}
	}

	newTarget := func(name string) ctypes.TypedObjectRef {
		return ctypes.TypedObjectRef{
			TypeMeta:  metav1.TypeMeta{Kind: "Subscription"},
			ObjectRef: ctypes.ObjectRef{Name: name, Namespace: testNamespace},
		}
	}

	newMeta := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    map[string]string{config.EnvironmentLabelKey: testEnvironment},
		}
	}

	It("rejects duplicate refs on an Approval status update", func() {
		const resourceName = "listmap-approval"
		namespacedName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}

		approval := &approvalv1.Approval{
			ObjectMeta: newMeta(resourceName),
			Spec: approvalv1.ApprovalSpec{
				Action:    "subscribe",
				Target:    newTarget(resourceName),
				Requester: newRequester(),
				Decider:   newDecider(),
				Strategy:  approvalv1.ApprovalStrategySimple,
				State:     approvalv1.ApprovalStateGranted,
				Decisions: []approvalv1.Decision{{
					Name:           "decider-0",
					Email:          "decider-0@example.com",
					ResultingState: approvalv1.ApprovalStateGranted,
				}},
			},
		}
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, approval)).To(Succeed())
		})

		var err error
		Eventually(func(g Gomega) {
			current := &approvalv1.Approval{}
			g.Expect(k8sClient.Get(ctx, namespacedName, current)).To(Succeed())
			current.Status.NotificationRefs = duplicateRefs
			err = k8sClient.Status().Update(ctx, current)
			g.Expect(apierrors.IsConflict(err)).To(BeFalse())
		}, timeout, interval).Should(Succeed())

		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid error, got %v", err)
	})

	It("rejects duplicate refs on an ApprovalRequest status update", func() {
		const resourceName = "listmap-approvalrequest"
		namespacedName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}

		approvalReq := &approvalv1.ApprovalRequest{
			ObjectMeta: newMeta(resourceName),
			Spec: approvalv1.ApprovalRequestSpec{
				Action:    "subscribe",
				Target:    newTarget(resourceName),
				Requester: newRequester(),
				Decider:   newDecider(),
				Strategy:  approvalv1.ApprovalStrategyFourEyes,
				State:     approvalv1.ApprovalStatePending,
			},
		}
		Expect(k8sClient.Create(ctx, approvalReq)).To(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, approvalReq)).To(Succeed())
		})

		var err error
		Eventually(func(g Gomega) {
			current := &approvalv1.ApprovalRequest{}
			g.Expect(k8sClient.Get(ctx, namespacedName, current)).To(Succeed())
			current.Status.NotificationRefs = duplicateRefs
			err = k8sClient.Status().Update(ctx, current)
			g.Expect(apierrors.IsConflict(err)).To(BeFalse())
		}, timeout, interval).Should(Succeed())

		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid error, got %v", err)
	})
})
