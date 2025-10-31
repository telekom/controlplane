// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateApproval(name string, arr *ctypes.ObjectRef) *approvalv1.Approval {
	appr := &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: approvalv1.ApprovalSpec{
			Strategy:        approvalv1.ApprovalStrategySimple,
			State:           approvalv1.ApprovalStatePending,
			ApprovedRequest: arr,
		},
	}

	err := k8sClient.Create(ctx, appr)
	Expect(err).ToNot(HaveOccurred())

	return appr
}

func ProgressApproval(appr *approvalv1.Approval, newState approvalv1.ApprovalState) *approvalv1.Approval {
	appr.Spec.State = newState
	err := k8sClient.Update(ctx, appr)
	Expect(err).ToNot(HaveOccurred())

	appr.Status.LastState = newState
	err = k8sClient.Status().Update(ctx, appr)
	Expect(err).ToNot(HaveOccurred())

	return appr
}

var _ = Describe("Approval Builder", Ordered, func() {

	var approvalName = "testresource--apisub"
	var approval *approvalv1.Approval

	requester := &approvalv1.Requester{
		Name:   "Max",
		Email:  "max.mustermann@telekom.de",
		Reason: "I need access to this API!!",
	}

	properties := map[string]any{
		"basePath": "/eni/distr/v1",
		"scopes":   "read",
	}

	AfterEach(func() {
		By("Deleting the ApprovalRequest")
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.ApprovalRequest{}, client.InNamespace(testNamespace))
		By("Deleting the Approval")
		_ = k8sClient.DeleteAllOf(ctx, &approvalv1.Approval{}, client.InNamespace(testNamespace))
	})

	Context("Approval does not exist", func() {

		It("should successfully create approvalRequest and set Owner conditions", func() {

			By("building the Approval")

			err := requester.SetProperties(properties)
			Expect(err).NotTo(HaveOccurred())

			jclient := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))

			owner := test.NewObject("apisub", testNamespace)
			owner.SetUID(types.UID("99d819b2-7dcb-41dd-abac-415719674737"))
			owner.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})

			builder := NewApprovalBuilder(jclient, owner)

			builder.WithHashValue(requester.Properties)
			builder.WithRequester(requester)
			builder.WithStrategy(approvalv1.ApprovalStrategyAuto)

			res, err := builder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(ApprovalResultPending)) // AR was just created, so pending

			Eventually(func(g Gomega) {
				ar := &approvalv1.ApprovalRequest{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      builder.GetApprovalRequest().Name,
					Namespace: testNamespace,
				}, ar)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(ar.Spec.Requester.Name).To(Equal("Max"))
				g.Expect(ar.Spec.Requester.Email).To(Equal("max.mustermann@telekom.de"))
				g.Expect(ar.Spec.Strategy).To(BeEquivalentTo("Auto"))
				g.Expect(ar.Spec.State).To(BeEquivalentTo("Granted"))

				processingCondition := meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeProcessing)
				readyCondition := meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeReady)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Reason).To(Equal("ApprovalPending"))
				g.Expect(readyCondition.Reason).To(Equal("ApprovalPending"))

				appr := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      builder.GetApproval().Name,
					Namespace: testNamespace,
				}, appr)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal("approvals.approval.cp.ei.telekom.de \"testresource--apisub\" not found"))

			}, timeout, interval).Should(Succeed())

		})
	})

	Context("Required Requester missing", func() {

		It("should fail creating the resources", func() {

			By("building the Approval without requester")

			err := requester.SetProperties(properties)
			Expect(err).NotTo(HaveOccurred())

			jclient := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))

			owner := test.NewObject("apisub", testNamespace)
			owner.SetUID(types.UID("99d819b2-7dcb-41dd-abac-415719674737"))
			owner.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})

			builder := NewApprovalBuilder(jclient, owner)

			builder.WithHashValue(requester.Properties)
			builder.WithStrategy(approvalv1.ApprovalStrategyAuto)

			res, err := builder.Build(ctx)

			Expect(err).To(HaveOccurred())

			Expect(res).To(BeEquivalentTo("None"))

		})
	})

	Context("Trusted Teams Auto-Approval", func() {
		It("should automatically set strategy to Auto when requester is from trusted team", func() {
			By("building the Approval with trusted teams")

			err := requester.SetProperties(properties)
			Expect(err).NotTo(HaveOccurred())

			jclient := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))

			owner := test.NewObject("apisub", testNamespace)
			owner.SetUID(types.UID("99d819b2-7dcb-41dd-abac-415719674737"))
			owner.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})

			builder := NewApprovalBuilder(jclient, owner)

			// Set up a requester that matches a trusted team
			requester := &approvalv1.Requester{
				Name:   "TrustedTeam",
				Email:  "trusted.team@telekom.de",
				Reason: "I need access to this API!!",
			}
			err = requester.SetProperties(properties)
			Expect(err).NotTo(HaveOccurred())

			// Configure the builder with trusted teams
			trustedTeams := []string{"TrustedTeam", "AnotherTeam"}

			builder.WithHashValue(requester.Properties)
			builder.WithRequester(requester)
			builder.WithTrustedRequesters(trustedTeams)

			// Set to Simple, but expect it to be overridden to Auto
			builder.WithStrategy(approvalv1.ApprovalStrategySimple)

			res, err := builder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(ApprovalResultPending))

			ar := &approvalv1.ApprovalRequest{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      builder.GetApprovalRequest().Name,
				Namespace: testNamespace,
			}, ar)
			Expect(err).ToNot(HaveOccurred())

			// Verify that the strategy was overridden to Auto
			Expect(ar.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategyAuto))
			Expect(ar.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		})
	})

	Context("Approval exists", func() {

		AfterEach(func() {
			By("Deleting the Approval")
			approval := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      approvalName,
					Namespace: testNamespace,
				},
			}
			err := k8sClient.Delete(ctx, approval)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle an already granted Approval", func() {
			jclient := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
			By("creating an approval-request")
			approvalRequestRef := &ctypes.ObjectRef{
				Name:      "apisub--5c65994fcc",
				Namespace: testNamespace,
			}

			approval = CreateApproval(approvalName, approvalRequestRef)
			ProgressApproval(approval, approvalv1.ApprovalStateGranted)

			By("creating a new approval request without any changes")
			err := requester.SetProperties(properties)
			Expect(err).NotTo(HaveOccurred())

			owner := test.NewObject("apisub", testNamespace)
			owner.SetUID(types.UID("99d819b2-7dcb-41dd-abac-415719674737"))
			owner.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})

			builder := NewApprovalBuilder(jclient, owner)

			builder.WithHashValue(requester.Properties)
			builder.WithRequester(requester)
			builder.WithStrategy(approvalv1.ApprovalStrategyAuto)
			builder.WithTrustedRequesters([]string{"IOnlyTrustThisRandomTeam"})

			res, err := builder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(ApprovalResultGranted)) // There were no changes and Approval is granted
		})

		It("should handle an already rejected Approval", func() {
			jclient := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
			By("creating an approval-request")
			approvalRequestRef := &ctypes.ObjectRef{
				Name:      "apisub--5c65994fdd",
				Namespace: testNamespace,
			}

			approval = CreateApproval(approvalName, approvalRequestRef)
			ProgressApproval(approval, approvalv1.ApprovalStateRejected)

			By("creating a new approval request without any changes")
			err := requester.SetProperties(properties)
			Expect(err).NotTo(HaveOccurred())

			owner := test.NewObject("apisub", testNamespace)
			owner.SetUID(types.UID("99d819b2-7dcb-41dd-abac-415719674737"))
			owner.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})

			builder := NewApprovalBuilder(jclient, owner)

			builder.WithHashValue(requester.Properties)
			builder.WithRequester(requester)
			builder.WithStrategy(approvalv1.ApprovalStrategyAuto)
			builder.WithTrustedRequesters([]string{"IOnlyTrustThisRandomTeam"})

			res, err := builder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(ApprovalResultDenied)) // There were no changes and Approval is granted

		})

		It("should ignore approvals for a different state", func() {
			jclient := cclient.NewJanitorClient(cclient.NewScopedClient(k8sm.GetClient(), testEnvironment))
			By("creating an approval-request")
			approvalRequestRef := &ctypes.ObjectRef{
				Name:      "apisub--5c65994fee",
				Namespace: testNamespace,
			}

			approval = CreateApproval(approvalName, approvalRequestRef)
			ProgressApproval(approval, approvalv1.ApprovalStateGranted)

			By("creating a new approval request with different hash (simulating a change)")
			err := requester.SetProperties(map[string]any{
				"basePath": "/eni/distr/v2", // changed basePath
				"scopes":   "read",
			})
			Expect(err).NotTo(HaveOccurred())

			owner := test.NewObject("apisub", testNamespace)
			owner.SetUID(types.UID("99d819b2-7dcb-41dd-abac-415719674737"))
			owner.SetLabels(map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			})

			builder := NewApprovalBuilder(jclient, owner)

			builder.WithHashValue(requester.Properties)
			builder.WithRequester(requester)
			builder.WithStrategy(approvalv1.ApprovalStrategyAuto)
			builder.WithTrustedRequesters([]string{"IOnlyTrustThisRandomTeam"})

			res, err := builder.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(ApprovalResultPending)) // There were changes, so pending
			processingCondition := meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCondition.Reason).To(Equal("Blocked"))
			Expect(processingCondition.Message).To(Equal("Approval is pending"))

			readyCondition := meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeReady)
			Expect(readyCondition.Reason).To(Equal("ApprovalPending"))

		})
	})

})
