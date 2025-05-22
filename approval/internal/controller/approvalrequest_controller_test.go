// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/types"
)

var _ = Describe("ApprovalRequest Controller", func() {

	ctx := context.Background()

	sourceResource := test.NewObject("my-test-resource", testNamespace)
	sourceResource.SetLabels(map[string]string{
		config.EnvironmentLabelKey: testEnvironment,
	})

	resquester := approvalv1.Requester{
		Name:  "test-requester",
		Email: "test@test.com",
		Properties: runtime.RawExtension{
			Raw: []byte(`{"scopes": ["test"]}`),
		},
	}

	arTempl := approvalv1.NewApprovalRequest(sourceResource, sourceResource.Spec)
	arTempl.SetLabels(map[string]string{
		config.EnvironmentLabelKey: testEnvironment,
	})

	Context("When reconciling a resource", func() {

		BeforeEach(func() {
			By("Creating the test resource")
			Expect(k8sClient.Create(ctx, sourceResource)).To(Succeed())
		})

		It("should automatically accept auto-approved approval-requests", func() {

			By("definitng the ApprovalRequest with auto strategy and granted state")
			ar := arTempl.DeepCopy()

			ar.Spec = approvalv1.ApprovalRequestSpec{
				Resource:  *types.TypedObjectRefFromObject(sourceResource, k8sClient.Scheme()),
				Requester: resquester,
				Strategy:  approvalv1.ApprovalStrategyAuto,
				State:     approvalv1.ApprovalStateGranted,
			}

			By("Creating the ApprovalRequest")
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			Eventually(func(g Gomega) {
				By("Checking the ApprovalRequest object")
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      ar.GetName(),
					Namespace: ar.GetNamespace(),
				}, ar)

				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(ar.Status.Approval.Name).To(Equal("testresource--my-test-resource"))
				g.Expect(ar.Spec.Requester.Properties.Raw).NotTo(BeNil())

				By("Checking the Approval object")
				a := &approvalv1.Approval{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "testresource--my-test-resource",
					Namespace: ar.GetNamespace(),
				}, a)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
				g.Expect(a.Spec.Requester.Properties.Raw).NotTo(BeNil())
				g.Expect(a.Spec.Requester.Name).To(Equal("test-requester"))
				g.Expect(a.Spec.Requester.Email).To(Equal("test@test.com"))

				processingCondition := meta.FindStatusCondition(a.Status.Conditions, condition.ConditionTypeProcessing)
				readyCondition := meta.FindStatusCondition(a.Status.Conditions, condition.ConditionTypeReady)

				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Message).To(Equal("Approval granted"))

				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(readyCondition.Message).To(Equal("Approval has been granted"))

				By("ApprovaRequest simmple:granted")
				ar.Spec.Strategy = approvalv1.ApprovalStrategySimple

				err = k8sClient.Update(ctx, ar)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(a.ObjectMeta.OwnerReferences).To(HaveLen(1))
				g.Expect(a.ObjectMeta.OwnerReferences[0].APIVersion).To(Equal("testgroup.cp.ei.telekom.de/v1"))
				g.Expect(a.ObjectMeta.OwnerReferences[0].Kind).To(Equal("TestResource"))
				g.Expect(a.ObjectMeta.OwnerReferences[0].Name).To(Equal("my-test-resource"))

				By("ApprovaRequest simmple:rejected")
				ar.Spec.State = approvalv1.ApprovalStateRejected

				err = k8sClient.Update(ctx, ar)
				g.Expect(err).NotTo(HaveOccurred())

				processingCondition = meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeProcessing)
				readyCondition = meta.FindStatusCondition(ar.Status.Conditions, condition.ConditionTypeReady)

				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Message).To(Equal("Request rejected"))

				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Message).To(Equal("Request has been rejected"))

			}, timeout, interval).Should(Succeed())
		})

	})
})
