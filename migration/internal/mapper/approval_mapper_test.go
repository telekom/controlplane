// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApprovalMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApprovalMapper Suite")
}

var _ = Describe("ApprovalMapper", func() {
	var (
		mapper          *ApprovalMapper
		ctx             context.Context
		approvalRequest *approvalv1.ApprovalRequest
	)

	BeforeEach(func() {
		mapper = NewApprovalMapper()
		ctx = context.Background()

		approvalRequest = &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-request",
				Namespace: "default",
			},
			Spec: approvalv1.ApprovalRequestSpec{
				State:    approvalv1.ApprovalStatePending,
				Strategy: approvalv1.ApprovalStrategySimple,
			},
		}
	})

	Describe("MapApprovalToRequest", func() {
		Context("with Granted state", func() {
			It("should map to Granted", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateGranted,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())
				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				Expect(approvalRequest.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategySimple))
			})
		})

		Context("with Rejected state", func() {
			It("should map to Rejected", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateRejected,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())
				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
			})
		})

		Context("with Suspended state", func() {
			It("should map to Rejected (special mapping)", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateSuspended,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())
				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
			})
		})

		Context("with Pending state", func() {
			It("should map to Pending", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStatePending,
						Strategy: approvalv1.ApprovalStrategyFourEyes,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())
				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStatePending))
				Expect(approvalRequest.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategyFourEyes))
			})
		})

		Context("with Semigranted state", func() {
			It("should map to Semigranted", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateSemigranted,
						Strategy: approvalv1.ApprovalStrategyFourEyes,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())
				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateSemigranted))
			})
		})

		Context("with annotations", func() {
			It("should add migration annotations", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateGranted,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())

				Expect(approvalRequest.Annotations).NotTo(BeNil())
				Expect(approvalRequest.Annotations).To(HaveKey("migration.cp.ei.telekom.de/migrated-from"))
				Expect(approvalRequest.Annotations).To(HaveKey("migration.cp.ei.telekom.de/migration-timestamp"))
				Expect(approvalRequest.Annotations).To(HaveKey("migration.cp.ei.telekom.de/last-migrated-state"))
				Expect(approvalRequest.Annotations).To(HaveKey("migration.cp.ei.telekom.de/legacy-state"))

				Expect(approvalRequest.Annotations["migration.cp.ei.telekom.de/migrated-from"]).To(Equal("legacy-approval"))
				Expect(approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"]).To(Equal("Granted"))
				Expect(approvalRequest.Annotations["migration.cp.ei.telekom.de/legacy-state"]).To(Equal("Granted"))
			})

			It("should preserve existing annotations", func() {
				approvalRequest.Annotations = map[string]string{
					"existing-key": "existing-value",
				}

				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateGranted,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())

				Expect(approvalRequest.Annotations["existing-key"]).To(Equal("existing-value"))
				Expect(approvalRequest.Annotations).To(HaveKey("migration.cp.ei.telekom.de/migrated-from"))
			})
		})

		Context("with Suspended to Rejected mapping", func() {
			It("should set legacy-state to Suspended but last-migrated-state to Rejected", func() {
				legacyApproval := &approvalv1.Approval{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-approval",
						Namespace: "default",
					},
					Spec: approvalv1.ApprovalSpec{
						State:    approvalv1.ApprovalStateSuspended,
						Strategy: approvalv1.ApprovalStrategySimple,
					},
				}

				err := mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval)
				Expect(err).NotTo(HaveOccurred())

				Expect(approvalRequest.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
				Expect(approvalRequest.Annotations["migration.cp.ei.telekom.de/legacy-state"]).To(Equal("Suspended"))
				Expect(approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"]).To(Equal("Rejected"))
			})
		})
	})

	Describe("MapState", func() {
		It("should map Suspended to Rejected", func() {
			result := mapper.MapState(approvalv1.ApprovalStateSuspended)
			Expect(result).To(Equal(approvalv1.ApprovalStateRejected))
		})

		It("should keep Granted as Granted", func() {
			result := mapper.MapState(approvalv1.ApprovalStateGranted)
			Expect(result).To(Equal(approvalv1.ApprovalStateGranted))
		})

		It("should keep Rejected as Rejected", func() {
			result := mapper.MapState(approvalv1.ApprovalStateRejected)
			Expect(result).To(Equal(approvalv1.ApprovalStateRejected))
		})

		It("should keep Pending as Pending", func() {
			result := mapper.MapState(approvalv1.ApprovalStatePending)
			Expect(result).To(Equal(approvalv1.ApprovalStatePending))
		})

		It("should keep Semigranted as Semigranted", func() {
			result := mapper.MapState(approvalv1.ApprovalStateSemigranted)
			Expect(result).To(Equal(approvalv1.ApprovalStateSemigranted))
		})
	})

	Describe("isValidState", func() {
		It("should accept valid states", func() {
			Expect(mapper.isValidState(approvalv1.ApprovalStatePending)).To(BeTrue())
			Expect(mapper.isValidState(approvalv1.ApprovalStateGranted)).To(BeTrue())
			Expect(mapper.isValidState(approvalv1.ApprovalStateRejected)).To(BeTrue())
			Expect(mapper.isValidState(approvalv1.ApprovalStateSuspended)).To(BeTrue())
			Expect(mapper.isValidState(approvalv1.ApprovalStateSemigranted)).To(BeTrue())
		})

		It("should reject empty state", func() {
			Expect(mapper.isValidState("")).To(BeFalse())
		})

		It("should reject invalid state", func() {
			Expect(mapper.isValidState("InvalidState")).To(BeFalse())
		})
	})

	Describe("isValidStrategy", func() {
		It("should accept Simple strategy", func() {
			Expect(mapper.isValidStrategy(approvalv1.ApprovalStrategySimple)).To(BeTrue())
		})

		It("should accept FourEyes strategy", func() {
			Expect(mapper.isValidStrategy(approvalv1.ApprovalStrategyFourEyes)).To(BeTrue())
		})

		It("should reject empty strategy", func() {
			Expect(mapper.isValidStrategy("")).To(BeFalse())
		})

		It("should reject invalid strategy", func() {
			Expect(mapper.isValidStrategy("InvalidStrategy")).To(BeFalse())
		})
	})
})
