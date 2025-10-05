// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ = Describe("NotificationChannel Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		notificationChannel := &notificationv1.NotificationChannel{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind NotificationChannel")
			err := k8sClient.Get(ctx, typeNamespacedName, notificationChannel)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.NotificationChannel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: notificationv1.NotificationChannelSpec{

						Email: &notificationv1.EmailConfig{
							Recipients:     []notificationv1.EmailString{"test@test.com"},
							CCRecipients:   nil,
							SMTPHost:       "someSMTPHost",
							SMTPPort:       1234,
							From:           "a@b",
							Authentication: nil,
						},
						Ignore: []string{"thisPurpose", "thatPurpose"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &notificationv1.NotificationChannel{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NotificationChannel")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			Eventually(func(g Gomega) {
				channel := &notificationv1.NotificationChannel{}
				err := k8sClient.Get(ctx, typeNamespacedName, channel)
				g.Expect(err).To(BeNil())

				g.Expect(channel.Spec.Email).ToNot(BeNil())
				g.Expect(channel.Spec.Email.From).To(Equal("a@b"))
				g.Expect(channel.Spec.Email.SMTPHost).To(Equal("someSMTPHost"))
				g.Expect(channel.Spec.Email.SMTPPort).To(Equal(1234))
				g.Expect(channel.Spec.Ignore).To(ContainElement("thisPurpose"))
				g.Expect(channel.Spec.Ignore).To(ContainElement("thatPurpose"))

				g.Expect(channel.Status.Conditions).To(HaveLen(2))
				g.Expect(meta.IsStatusConditionTrue(channel.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(channel.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})
