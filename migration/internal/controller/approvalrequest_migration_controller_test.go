// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Mock Handler
type mockHandler struct {
	handleFunc func(ctx context.Context, approvalRequest *approvalv1.ApprovalRequest) error
	callCount  int
}

func (m *mockHandler) Handle(ctx context.Context, approvalRequest *approvalv1.ApprovalRequest) error {
	m.callCount++
	if m.handleFunc != nil {
		return m.handleFunc(ctx, approvalRequest)
	}
	return nil
}

var _ = Describe("ApprovalRequestMigrationReconciler", func() {
	var (
		reconciler      *ApprovalRequestMigrationReconciler
		handler         *mockHandler
		approvalRequest *approvalv1.ApprovalRequest
		ctx             context.Context
		scheme          *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		handler = &mockHandler{}

		scheme = runtime.NewScheme()
		Expect(approvalv1.AddToScheme(scheme)).To(Succeed())

		approvalRequest = &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-request",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "api.cp.ei.telekom.de/v1",
						Kind:       "APISubscription",
						Name:       "test-subscription",
						UID:        "12345",
					},
				},
			},
			Spec: approvalv1.ApprovalRequestSpec{
				State:    approvalv1.ApprovalStatePending,
				Strategy: approvalv1.ApprovalStrategySimple,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(approvalRequest).
			Build()

		reconciler = NewApprovalRequestMigrationReconciler(
			fakeClient,
			scheme,
			handler,
			logr.Discard(),
		)
	})

	Describe("Reconcile", func() {
		Context("when ApprovalRequest exists", func() {
			It("should call handler and requeue", func() {
				handler.handleFunc = func(ctx context.Context, ar *approvalv1.ApprovalRequest) error {
					Expect(ar.Name).To(Equal("test-request"))
					Expect(ar.Namespace).To(Equal("default"))
					return nil
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-request",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.callCount).To(Equal(1))
				Expect(result.RequeueAfter).To(Equal(RequeueAfterDuration))
			})
		})

		Context("when ApprovalRequest does not exist", func() {
			It("should not error", func() {
				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "nonexistent",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
				Expect(handler.callCount).To(Equal(0))
			})
		})

		Context("when handler returns error", func() {
			It("should requeue with error", func() {
				handler.handleFunc = func(ctx context.Context, ar *approvalv1.ApprovalRequest) error {
					return errors.New("migration failed")
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-request",
						Namespace: "default",
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("migration failed"))
				Expect(result.RequeueAfter).To(Equal(RequeueAfterDuration))
			})
		})

		Context("with multiple reconciliations", func() {
			It("should call handler each time", func() {
				handler.handleFunc = func(ctx context.Context, ar *approvalv1.ApprovalRequest) error {
					return nil
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "test-request",
						Namespace: "default",
					},
				}

				// First reconciliation
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.callCount).To(Equal(1))

				// Second reconciliation
				result, err = reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.callCount).To(Equal(2))
				Expect(result.RequeueAfter).To(Equal(RequeueAfterDuration))
			})
		})
	})

	Describe("RequeueAfterDuration", func() {
		It("should be 30 seconds", func() {
			Expect(RequeueAfterDuration).To(Equal(30 * time.Second))
		})
	})
})
