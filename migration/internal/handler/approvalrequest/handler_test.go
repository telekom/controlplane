// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/migration/internal/mapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMigrationHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MigrationHandler Suite")
}

// Mock Remote Client
type mockRemoteClient struct {
	getApprovalFunc func(ctx context.Context, namespace, name string) (*approvalv1.Approval, error)
}

func (m *mockRemoteClient) GetApproval(ctx context.Context, namespace, name string) (*approvalv1.Approval, error) {
	if m.getApprovalFunc != nil {
		return m.getApprovalFunc(ctx, namespace, name)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRemoteClient) ListApprovals(ctx context.Context, namespace string) (*approvalv1.ApprovalList, error) {
	return nil, errors.New("not implemented")
}

var _ = Describe("MigrationHandler", func() {
	var (
		handler         *MigrationHandler
		mockClient      *mockRemoteClient
		approvalRequest *approvalv1.ApprovalRequest
		ctx             context.Context
		scheme          *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = &mockRemoteClient{}

		scheme = runtime.NewScheme()
		Expect(approvalv1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		handler = NewMigrationHandler(
			fakeClient,
			mockClient,
			mapper.NewApprovalMapper(),
			logr.Discard(),
		)

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
	})

	Describe("computeLegacyApprovalName", func() {
		It("should compute name from owner reference", func() {
			name, err := handler.computeLegacyApprovalName(approvalRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("apisubscription--test-subscription"))
		})

		It("should return empty string when no owner references", func() {
			approvalRequest.OwnerReferences = []metav1.OwnerReference{}
			name, err := handler.computeLegacyApprovalName(approvalRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal(""))
		})

		It("should handle different kinds", func() {
			approvalRequest.OwnerReferences[0].Kind = "APIKey"
			approvalRequest.OwnerReferences[0].Name = "my-key"

			name, err := handler.computeLegacyApprovalName(approvalRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("apikey--my-key"))
		})
	})

	Describe("hasStateChanged", func() {
		Context("when no migration annotation exists", func() {
			It("should return true (first migration)", func() {
				changed := handler.hasStateChanged(approvalRequest, approvalv1.ApprovalStateGranted)
				Expect(changed).To(BeTrue())
			})
		})

		Context("when annotation exists and state matches", func() {
			It("should return false", func() {
				approvalRequest.Annotations = map[string]string{
					"migration.cp.ei.telekom.de/last-migrated-state": "Granted",
				}

				changed := handler.hasStateChanged(approvalRequest, approvalv1.ApprovalStateGranted)
				Expect(changed).To(BeFalse())
			})
		})

		Context("when annotation exists and state differs", func() {
			It("should return true", func() {
				approvalRequest.Annotations = map[string]string{
					"migration.cp.ei.telekom.de/last-migrated-state": "Pending",
				}

				changed := handler.hasStateChanged(approvalRequest, approvalv1.ApprovalStateGranted)
				Expect(changed).To(BeTrue())
			})
		})

		Context("with Suspended to Rejected mapping", func() {
			It("should compare mapped state", func() {
				approvalRequest.Annotations = map[string]string{
					"migration.cp.ei.telekom.de/last-migrated-state": "Rejected",
				}

				// Legacy state is Suspended, which maps to Rejected
				changed := handler.hasStateChanged(approvalRequest, approvalv1.ApprovalStateSuspended)
				Expect(changed).To(BeFalse()) // Should not change because Suspended maps to Rejected
			})

			It("should detect change when going from other state to Suspended", func() {
				approvalRequest.Annotations = map[string]string{
					"migration.cp.ei.telekom.de/last-migrated-state": "Granted",
				}

				// Legacy state is now Suspended (maps to Rejected)
				changed := handler.hasStateChanged(approvalRequest, approvalv1.ApprovalStateSuspended)
				Expect(changed).To(BeTrue())
			})
		})
	})

	Describe("Handle", func() {
		Context("when legacy approval exists with different state", func() {
			It("should update the approval request", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apisubscription--test-subscription",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateGranted,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				mockClient.getApprovalFunc = func(ctx context.Context, namespace, name string) (*approvalv1.Approval, error) {
					Expect(namespace).To(Equal("default"))
					Expect(name).To(Equal("apisubscription--test-subscription"))
					return legacyApproval, nil
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(approvalRequest).
					Build()

				handler.Client = fakeClient

				err := handler.Handle(ctx, approvalRequest)
				Expect(err).NotTo(HaveOccurred())

				// Verify state was updated
				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))

				// Verify annotations were added
				Expect(approvalRequest.Annotations).To(HaveKey("migration.cp.ei.telekom.de/migrated-from"))
				Expect(approvalRequest.Annotations).To(HaveKey("migration.cp.ei.telekom.de/last-migrated-state"))
			})
		})

		Context("when legacy approval has Suspended state", func() {
			It("should map to Rejected", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apisubscription--test-subscription",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateSuspended,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				mockClient.getApprovalFunc = func(ctx context.Context, namespace, name string) (*approvalv1.Approval, error) {
					return legacyApproval, nil
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(approvalRequest).
					Build()

				handler.Client = fakeClient

				err := handler.Handle(ctx, approvalRequest)
				Expect(err).NotTo(HaveOccurred())

				// Verify state was mapped to Rejected
				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))

				// Verify annotations show the mapping
				Expect(approvalRequest.Annotations["migration.cp.ei.telekom.de/legacy-state"]).To(Equal("Suspended"))
				Expect(approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"]).To(Equal("Rejected"))
			})
		})

		Context("when legacy approval does not exist", func() {
			It("should skip without error", func() {
				mockClient.getApprovalFunc = func(ctx context.Context, namespace, name string) (*approvalv1.Approval, error) {
					return nil, errors.New("not found")
				}

				err := handler.Handle(ctx, approvalRequest)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when no owner references exist", func() {
			It("should skip without error", func() {
				approvalRequest.OwnerReferences = []metav1.OwnerReference{}

				err := handler.Handle(ctx, approvalRequest)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when state has not changed", func() {
			It("should skip update", func() {
				approvalRequest.Annotations = map[string]string{
					"migration.cp.ei.telekom.de/last-migrated-state": "Granted",
				}

				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apisubscription--test-subscription",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateGranted,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				mockClient.getApprovalFunc = func(ctx context.Context, namespace, name string) (*approvalv1.Approval, error) {
					return legacyApproval, nil
				}

				initialState := approvalRequest.Spec.State

				err := handler.Handle(ctx, approvalRequest)
				Expect(err).NotTo(HaveOccurred())

				// State should not have changed
				Expect(approvalRequest.Spec.State).To(Equal(initialState))
			})
		})

		Context("when remote client returns unexpected error", func() {
			It("should return error", func() {
				mockClient.getApprovalFunc = func(ctx context.Context, namespace, name string) (*approvalv1.Approval, error) {
					return nil, errors.New("connection timeout")
				}

				err := handler.Handle(ctx, approvalRequest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection timeout"))
			})
		})
	})
})
