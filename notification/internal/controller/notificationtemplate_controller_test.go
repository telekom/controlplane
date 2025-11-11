// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ = Describe("NotificationTemplate Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		notificationTemplate := &notificationv1.NotificationTemplate{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind NotificationTemplate")
			err := k8sClient.Get(ctx, typeNamespacedName, notificationTemplate)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.NotificationTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},

					Spec: notificationv1.NotificationTemplateSpec{
						Purpose:         "ApiSubscriptionApproved",
						ChannelType:     "Email",
						SubjectTemplate: "test-subjectTemplate",
						Template:        "test-template",
						Schema:          runtime.RawExtension{},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &notificationv1.NotificationTemplate{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NotificationTemplate")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Checking if the Notification template is created and all conditions are set")

			Eventually(func(g Gomega) {
				template := &notificationv1.NotificationTemplate{}
				err := k8sClient.Get(ctx, typeNamespacedName, template)
				g.Expect(err).To(BeNil())

				g.Expect(template.Spec.Purpose).To(Equal("ApiSubscriptionApproved"))
				g.Expect(template.Spec.ChannelType).To(Equal("Email"))
				g.Expect(template.Spec.SubjectTemplate).To(Equal("test-subjectTemplate"))
				g.Expect(template.Spec.Template).To(Equal("test-template"))
				g.Expect(template.Spec.Schema).To(Equal(runtime.RawExtension{}))

				g.Expect(template.Status.Conditions).To(HaveLen(2))
				g.Expect(meta.IsStatusConditionTrue(template.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(template.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})
