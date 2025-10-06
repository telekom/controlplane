// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commontypes "github.com/telekom/controlplane/common/pkg/types"

	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ = Describe("Notification Controller", func() {
	Context("When reconciling a resource", func() {
		const notificationPurpose = "test-purpose"

		const notificationName = "test-notification"
		// per convention
		const templateName = "template--" + notificationPurpose + "--mail"
		const channelName = "channel--eni--hyperion--mail"

		ctx := context.Background()

		notificationNamespacedName := types.NamespacedName{
			Name:      notificationName,
			Namespace: "default",
		}
		notification := &notificationv1.Notification{}

		templateNamespacedName := types.NamespacedName{
			Name:      templateName,
			Namespace: "default",
		}
		template := &notificationv1.NotificationTemplate{}

		channelNamespacedName := types.NamespacedName{
			Name:      channelName,
			Namespace: "default",
		}
		channel := &notificationv1.NotificationChannel{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Notification Template")
			err := k8sClient.Get(ctx, templateNamespacedName, template)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.NotificationTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      templateName,
						Namespace: "default",
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: notificationv1.NotificationTemplateSpec{
						Purpose:         notificationPurpose,
						ChannelType:     "Email",
						SubjectTemplate: "Subject: {{.subjectValue}}\n",
						Template:        "Body: {{.bodyValue}}\n",
						Schema:          runtime.RawExtension{},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind Notification Channel")
			err = k8sClient.Get(ctx, channelNamespacedName, channel)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.NotificationChannel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      channelName,
						Namespace: "default",
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: notificationv1.NotificationChannelSpec{
						Email: &notificationv1.EmailConfig{
							Recipients:     []notificationv1.EmailString{"john.doe@example.com"},
							CCRecipients:   nil,
							SMTPHost:       "testSMTPHost",
							SMTPPort:       1234,
							From:           "test.from@somewhere.test",
							Authentication: nil,
						},
						MsTeams: nil,
						Webhook: nil,
						Ignore:  nil,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind Notification")
			err = k8sClient.Get(ctx, notificationNamespacedName, notification)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.Notification{
					ObjectMeta: metav1.ObjectMeta{
						Name:      notificationName,
						Namespace: "default",
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: notificationv1.NotificationSpec{
						Purpose: notificationPurpose,
						Sender: notificationv1.Sender{
							Type: notificationv1.SenderTypeUser,
							Name: "John Snow",
						},
						Channels: []commontypes.ObjectRef{
							commontypes.ObjectRef{
								Name:      channelName,
								Namespace: "default",
							},
						},
						Properties: runtime.RawExtension{
							Raw: []byte(`{"subjectValue":"awesomeSubject", "bodyValue":"awesomeBody"}`),
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &notificationv1.Notification{}
			err := k8sClient.Get(ctx, notificationNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Notification")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource with no channels - nothing to do", func() {
			By("Reconciling the created resource")

			Eventually(func(g Gomega) {
				notification := &notificationv1.Notification{}
				err := k8sClient.Get(ctx, notificationNamespacedName, notification)
				g.Expect(err).To(BeNil())

				g.Expect(notification.Spec.Purpose).To(Equal("test-purpose"))
				g.Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeUser))
				g.Expect(notification.Spec.Sender.Name).To(Equal("John Snow"))
				g.Expect(notification.Spec.Channels).To(Not(BeEmpty()))

				// to be able to compare jsons with different order of fields
				ExpectJSONEqual(notification.Spec.Properties.Raw, []byte(`{"subjectValue":"awesomeSubject", "bodyValue":"awesomeBody"}`))

				g.Expect(notification.Status.Conditions).To(HaveLen(2))
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

				// notifications are sent
				g.Expect(notification.Status.States).To(HaveLen(1))

				g.Expect(notification.Status.States).To(HaveKey("default/channel--eni--hyperion--mail"))

				// omit the timestamp as its dynamic
				g.Expect(notification.Status.States["default/channel--eni--hyperion--mail"]).To(Not(BeNil()))
				g.Expect(notification.Status.States["default/channel--eni--hyperion--mail"].Sent).To(BeTrue())
				g.Expect(notification.Status.States["default/channel--eni--hyperion--mail"].ErrorMessage).To(BeEquivalentTo("Successfully sent"))
			}, timeout, interval).Should(Succeed())

		})
	})
})

func ExpectJSONEqual(actualJSON, expectedJSON []byte) {
	var actual, expected map[string]interface{}
	Expect(json.Unmarshal(actualJSON, &actual)).To(Succeed())
	Expect(json.Unmarshal(expectedJSON, &expected)).To(Succeed())
	Expect(actual).To(Equal(expected))
}
