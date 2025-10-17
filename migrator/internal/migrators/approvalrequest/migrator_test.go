// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApprovalRequestMigrator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApprovalRequest Migrator Suite")
}

var _ = Describe("ApprovalRequestMigrator", func() {
	var (
		migrator        *ApprovalRequestMigrator
		approvalRequest *approvalv1.ApprovalRequest
		ctx             context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		migrator = NewApprovalRequestMigrator()

		approvalRequest = &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-request",
				Namespace: "controlplane--eni--hyperion",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "rover.cp.ei.telekom.de/v1",
						Kind:       "ApiSubscription",
						Name:       "consumer--api-name",
						UID:        "12345",
					},
				},
			},
			Spec: approvalv1.ApprovalRequestSpec{
				State: approvalv1.ApprovalStatePending,
			},
		}
	})

	Describe("GetName", func() {
		It("should return 'approvalrequest'", func() {
			Expect(migrator.GetName()).To(Equal("approvalrequest"))
		})
	})

	Describe("GetNewResourceType", func() {
		It("should return ApprovalRequest type", func() {
			obj := migrator.GetNewResourceType()
			Expect(obj).To(BeAssignableToTypeOf(&approvalv1.ApprovalRequest{}))
		})
	})

	Describe("GetLegacyAPIGroup", func() {
		It("should return legacy API group", func() {
			Expect(migrator.GetLegacyAPIGroup()).To(Equal("acp.ei.telekom.de"))
		})
	})

	Describe("ComputeLegacyIdentifier", func() {
		It("should compute namespace and name correctly", func() {
			namespace, name, skip, err := migrator.ComputeLegacyIdentifier(ctx, approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
			Expect(namespace).To(Equal("eni--hyperion"))
			Expect(name).To(Equal("apisubscription--api-name--consumer"))
		})

		It("should skip when no owner references", func() {
			approvalRequest.OwnerReferences = []metav1.OwnerReference{}

			namespace, name, skip, err := migrator.ComputeLegacyIdentifier(ctx, approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeTrue())
			Expect(namespace).To(BeEmpty())
			Expect(name).To(BeEmpty())
		})

		It("should handle simple names without swapping", func() {
			approvalRequest.OwnerReferences[0].Name = "simple-name"

			namespace, name, skip, err := migrator.ComputeLegacyIdentifier(ctx, approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
			Expect(namespace).To(Equal("eni--hyperion"))
			Expect(name).To(Equal("apisubscription--simple-name"))
		})

		It("should strip environment from namespace", func() {
			approvalRequest.Namespace = "production--group--team"

			namespace, _, skip, err := migrator.ComputeLegacyIdentifier(ctx, approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
			Expect(namespace).To(Equal("group--team"))
		})

		It("should handle namespace without environment prefix", func() {
			approvalRequest.Namespace = "default"

			namespace, _, skip, err := migrator.ComputeLegacyIdentifier(ctx, approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(skip).To(BeFalse())
			Expect(namespace).To(Equal("default"))
		})
	})

	Describe("computeLegacyApprovalName", func() {
		It("should swap components in owner name", func() {
			approvalRequest.OwnerReferences[0].Name = "rover--api"

			name, err := migrator.computeLegacyApprovalName(approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("apisubscription--api--rover"))
		})

		It("should handle complex names with multiple dashes", func() {
			approvalRequest.OwnerReferences[0].Name = "manual-tests-consumer--eni-manual-tests-echo-v1"

			name, err := migrator.computeLegacyApprovalName(approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("apisubscription--eni-manual-tests-echo-v1--manual-tests-consumer"))
		})

		It("should convert kind to lowercase", func() {
			approvalRequest.OwnerReferences[0].Kind = "APISubscription"
			approvalRequest.OwnerReferences[0].Name = "test"

			name, err := migrator.computeLegacyApprovalName(approvalRequest)

			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("apisubscription--test"))
		})
	})

	Describe("computeLegacyNamespace", func() {
		It("should strip environment prefix", func() {
			namespace := migrator.computeLegacyNamespace("controlplane--eni--hyperion")
			Expect(namespace).To(Equal("eni--hyperion"))
		})

		It("should return as-is if no separator", func() {
			namespace := migrator.computeLegacyNamespace("default")
			Expect(namespace).To(Equal("default"))
		})

		It("should handle different environments", func() {
			namespace := migrator.computeLegacyNamespace("production--phoenix--firebirds")
			Expect(namespace).To(Equal("phoenix--firebirds"))
		})
	})

	Describe("HasChanged", func() {
		var approval *approvalv1.Approval

		BeforeEach(func() {
			approval = &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					State: approvalv1.ApprovalStateGranted,
				},
			}
		})

		It("should return true when no migration annotation exists", func() {
			changed := migrator.HasChanged(ctx, approvalRequest, approval)
			Expect(changed).To(BeTrue())
		})

		It("should return false when state matches annotation", func() {
			approvalRequest.Annotations = map[string]string{
				"migration.cp.ei.telekom.de/last-migrated-state": "Granted",
			}

			changed := migrator.HasChanged(ctx, approvalRequest, approval)
			Expect(changed).To(BeFalse())
		})

		It("should return true when state differs from annotation", func() {
			approvalRequest.Annotations = map[string]string{
				"migration.cp.ei.telekom.de/last-migrated-state": "Pending",
			}

			changed := migrator.HasChanged(ctx, approvalRequest, approval)
			Expect(changed).To(BeTrue())
		})

		It("should handle Suspended to Rejected mapping", func() {
			approval.Spec.State = approvalv1.ApprovalStateSuspended
			approvalRequest.Annotations = map[string]string{
				"migration.cp.ei.telekom.de/last-migrated-state": "Rejected",
			}

			changed := migrator.HasChanged(ctx, approvalRequest, approval)
			Expect(changed).To(BeFalse())
		})
	})

	Describe("ApplyMigration", func() {
		var approval *approvalv1.Approval

		BeforeEach(func() {
			approval = &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legacy-approval",
					Namespace: "eni--hyperion",
				},
				Spec: approvalv1.ApprovalSpec{
					State:    approvalv1.ApprovalStateGranted,
					Strategy: approvalv1.ApprovalStrategySimple,
				},
			}
		})

		It("should apply migration successfully", func() {
			err := migrator.ApplyMigration(ctx, approvalRequest, approval)

			Expect(err).NotTo(HaveOccurred())
			Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		})

		It("should update state from Pending to Granted", func() {
			approvalRequest.Spec.State = approvalv1.ApprovalStatePending
			approval.Spec.State = approvalv1.ApprovalStateGranted

			err := migrator.ApplyMigration(ctx, approvalRequest, approval)

			Expect(err).NotTo(HaveOccurred())
			Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		})
	})

	Describe("GetRequeueAfter", func() {
		It("should return 30 seconds", func() {
			Expect(migrator.GetRequeueAfter().Seconds()).To(Equal(30.0))
		})
	})
})
