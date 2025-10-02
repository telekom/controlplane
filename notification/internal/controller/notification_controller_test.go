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

var _ = Describe("Notification Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		notification := &notificationv1.Notification{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Notification")
			err := k8sClient.Get(ctx, typeNamespacedName, notification)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.Notification{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: notificationv1.NotificationSpec{
						Purpose: "thisPurpose",
						Sender: notificationv1.Sender{
							Type: notificationv1.SenderTypeUser,
							Name: "John Snow",
						},
						Channels: nil,
						Properties: runtime.RawExtension{
							Raw: []byte(`{"placeholder1":"value1"}`),
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &notificationv1.Notification{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Notification")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			Eventually(func(g Gomega) {
				notification := &notificationv1.Notification{}
				err := k8sClient.Get(ctx, typeNamespacedName, notification)
				g.Expect(err).To(BeNil())

				g.Expect(notification.Spec.Purpose).To(Equal("thisPurpose"))
				g.Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeUser))
				g.Expect(notification.Spec.Sender.Name).To(Equal("John Snow"))
				g.Expect(notification.Spec.Channels).To(BeEmpty())
				g.Expect(notification.Spec.Properties.Raw).To(Equal([]byte(`{"placeholder1":"value1"}`)))

				g.Expect(notification.Status.Conditions).To(HaveLen(2))
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

		})
	})
})
