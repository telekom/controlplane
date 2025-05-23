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
			Strategy:        approvalv1.ApprovalStrategyAuto,
			State:           approvalv1.ApprovalStateGranted,
			ApprovedRequest: arr,
		},
	}

	err := k8sClient.Create(ctx, appr)
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

	AfterAll(func() {
		By("Deleting the Approval")
		err := k8sClient.Delete(ctx, approval)
		Expect(err).ToNot(HaveOccurred())
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
			Expect(res).To(Equal(ApprovalResultPending))

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

	Context("Approval exists", func() {

		It("should successfully set Owner conditions", func() {

			By("creating Approval")

			arr := &ctypes.ObjectRef{
				Name:      "apisub--5c65994fcc",
				Namespace: testNamespace,
			}

			approval = CreateApproval(approvalName, arr)

			By("Approval Granted")

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
			// This check is subject to a race condition.
			// The mutator hook within the builder is not run at time of this check.
			// Therefore, the changes within the mutator are not _always_ in the mocked k8s cluster.
			// About 1 out of 5 runs, the mutator is not able to change the State to Granted due to the ApprovalStrategyAuto.
			// Adding k8sm.GetCache().WaitForCacheSync(ctx)) might not work, since the k8sClient.Get is run within the builder.
			Expect(k8sm.GetCache().WaitForCacheSync(ctx)).To(BeTrue(), "cache not synced")
			Expect(res).To(Equal(ApprovalResultGranted))

			Eventually(func(g Gomega) {

				processingCondition := meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeProcessing)
				readyCondition := meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeReady)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(processingCondition.Reason).To(Equal("ApprovalGranted"))
				g.Expect(readyCondition.Reason).To(Equal("ApprovalGranted"))

				appr := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      builder.GetApproval().Name,
					Namespace: testNamespace,
				}, appr)
				g.Expect(err).ToNot(HaveOccurred())

				By("Approval Rejected")
				appr.Spec.State = approvalv1.ApprovalStateRejected

				err = k8sClient.Update(ctx, appr)
				g.Expect(err).ToNot(HaveOccurred())

				builder := NewApprovalBuilder(jclient, owner)
				builder.WithHashValue(requester.Properties)
				builder.WithRequester(requester)
				builder.WithStrategy(approvalv1.ApprovalStrategyAuto)

				res, err := builder.Build(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(ApprovalResultDenied))

				processingCondition = meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeProcessing)
				readyCondition = meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeReady)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Reason).To(Equal("Blocked"))
				g.Expect(readyCondition.Message).To(Equal("Approval is either rejected or suspended"))

				By("Different ApprovalReq. Name")

				appr.Spec.ApprovedRequest.Name = "apisub--5c65994fdd"
				err = k8sClient.Update(ctx, appr)
				g.Expect(err).ToNot(HaveOccurred())

				builder = NewApprovalBuilder(jclient, owner)
				builder.WithHashValue(requester.Properties)
				builder.WithRequester(requester)
				builder.WithStrategy(approvalv1.ApprovalStrategyAuto)

				res, err = builder.Build(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(ApprovalResultDenied))

				processingCondition = meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeProcessing)
				readyCondition = meta.FindStatusCondition(builder.GetOwner().GetConditions(), condition.ConditionTypeReady)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Reason).To(Equal("Blocked"))
				g.Expect(readyCondition.Message).To(Equal("Approval is either rejected or suspended"))

			}, timeout, interval).Should(Succeed())

		})
	})

})
