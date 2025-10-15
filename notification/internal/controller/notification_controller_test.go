// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	mailsender "github.com/telekom/controlplane/notification/internal/sender/adapter/mail"
	mailsendermock "github.com/telekom/controlplane/notification/internal/sender/adapter/mail/mock"

	notificationconfig "github.com/telekom/controlplane/notification/internal/config"
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
			Namespace: testEnvironment,
		}
		template := &notificationv1.NotificationTemplate{}

		channelNamespacedName := types.NamespacedName{
			Name:      channelName,
			Namespace: "default",
		}
		channel := &notificationv1.NotificationChannel{}

		BeforeEach(func() {
			By("Mocking the actual email sender")
			mockMailSender := &mailsendermock.MockEmailSender{}
			//mockMailSender.EXPECT().Send(ctx, "test.from@somewhere.test", "Team Tardis", []string{"john.doe@example.com"}, "Subject: awesomeSubject", "Body: awesomeBody").Return(nil)
			mockMailSender.EXPECT().Send(mock.Anything, "test.from@somewhere.test", "Team Tardis", []string{"john.doe@example.com"}, "Subject: awesomeSubject\n", "Body: awesomeBody\n").Return(nil)

			mailsender.NewSMTPSender = func(config *notificationconfig.EmailAdapterConfig) mailsender.EmailSender {
				return mockMailSender
			}

			By("creating the custom resource for the Kind Notification Template")
			err := k8sClient.Get(ctx, templateNamespacedName, template)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.NotificationTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      templateName,
						Namespace: testEnvironment,
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
				fromString := "test.from@somewhere.test"
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
							Recipients:     []string{"john.doe@example.com"},
							From:           &fromString,
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
							{
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
			By("Cleanup the specific resource instance Notification")
			notificationResource := &notificationv1.Notification{}
			err := k8sClient.Get(ctx, notificationNamespacedName, notificationResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, notificationResource)).To(Succeed())

			By("Cleanup the specific resource instance Notification channel")
			channelResource := &notificationv1.NotificationChannel{}
			err = k8sClient.Get(ctx, channelNamespacedName, channelResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, channelResource)).To(Succeed())

			By("Cleanup the specific resource instance Notification template")
			templateResource := &notificationv1.NotificationTemplate{}
			err = k8sClient.Get(ctx, templateNamespacedName, templateResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, templateResource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Sending the notification per channel")

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

	Context("When reconciling a resource and some resources are missing", func() {
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
			Namespace: testEnvironment,
		}
		template := &notificationv1.NotificationTemplate{}

		AfterEach(func() {
			resource := &notificationv1.Notification{}
			err := k8sClient.Get(ctx, notificationNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Notification")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should fail to reconcile the notification if a channel is missing", func() {

			By("creating the custom resource for the Kind Notification Template")
			err := k8sClient.Get(ctx, templateNamespacedName, template)
			if err != nil && errors.IsNotFound(err) {
				resource := &notificationv1.NotificationTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      templateName,
						Namespace: testEnvironment,
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
							{
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
			By("not creating the custom resource for the Kind Notification channel")

			By("Updating the status.states with a failed SendState")

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
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeFalse())

				// notifications are sent
				g.Expect(notification.Status.States).To(HaveLen(1))

				g.Expect(notification.Status.States).To(HaveKey("default/channel--eni--hyperion--mail"))

				// omit the timestamp as its dynamic
				g.Expect(notification.Status.States["default/channel--eni--hyperion--mail"]).To(Not(BeNil()))
				g.Expect(notification.Status.States["default/channel--eni--hyperion--mail"].Sent).To(BeFalse())
				g.Expect(notification.Status.States["default/channel--eni--hyperion--mail"].ErrorMessage).To(BeEquivalentTo("Error getting channel \"default/channel--eni--hyperion--mail\": failed to get object: NotificationChannel.notification.cp.ei.telekom.de \"channel--eni--hyperion--mail\" not found"))
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
