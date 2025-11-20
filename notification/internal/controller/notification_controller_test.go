// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	notificationconfig "github.com/telekom/controlplane/notification/internal/config"
	mailsender "github.com/telekom/controlplane/notification/internal/sender/adapter/mail"
	mailsendermock "github.com/telekom/controlplane/notification/internal/sender/adapter/mail/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"math/rand/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	//ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	notificationPurpose = "test-purpose"
	defaultNamespace    = "default"
)

var _ = Describe("Notification Controller", Ordered, func() {

	Context("When reconciling a resource", func() {
		var (
			ctx              context.Context
			templateName     string
			channelName      string
			notificationName string
			purposeName      string

			notif    *notificationv1.Notification
			template *notificationv1.NotificationTemplate
			channel  *notificationv1.NotificationChannel
		)

		BeforeEach(func() {
			ctx = context.Background()

			By("Mocking the actual email sender")
			mockMailSender := &mailsendermock.MockEmailSender{}
			mockMailSender.EXPECT().Send(mock.Anything, "test.from@somewhere.test", "Team Tardis", []string{"john.doe@example.com"}, "Subject: awesomeSubject\n", "Body: awesomeBody\n").Return(nil)

			mailsender.NewSMTPSender = func(config *notificationconfig.EmailAdapterConfig) mailsender.EmailSender {
				return mockMailSender
			}

			By("creating a new purpose name")
			purposeName = randName("purpose")

			By("creating the custom resource for the Kind Notification Template")
			// tie this to the notification purpose, as we use template name resolution via convention
			// purpose-12351--mail is valid
			// purpose--mail-12351 is not valid
			templateName = purposeName + "--mail"
			template = newTestNotificationTemplate(ctx, templateName, testEnvironment)

			By("creating the custom resource for the Kind Notification Channel")
			channelName = randName("eni--hyperion--mail")
			channel = newTestNotificationChannel(ctx, channelName, defaultNamespace)

			By("creating the custom resource for the Kind Notification")
			notificationName = randName("notification")
			notif = newTestNotification(ctx, notificationName, defaultNamespace, withChannel(channelName, defaultNamespace), withPurpose(purposeName))
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance Notification")
			deleteIfExists(ctx, notif)

			By("Cleanup the specific resource instance Notification channel")
			deleteIfExists(ctx, channel)

			By("Cleanup the specific resource instance Notification template")
			deleteIfExists(ctx, template)
		})

		It("should successfully reconcile the resource", func() {
			By("Sending the notification per channel")

			Eventually(func(g Gomega) {
				notification := &notificationv1.Notification{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notificationName,
					Namespace: defaultNamespace,
				}, notification)
				g.Expect(err).To(BeNil())

				g.Expect(notification.Spec.Purpose).To(Equal(purposeName))
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

				channelMapKey := defaultNamespace + "/" + channelName
				g.Expect(notification.Status.States).To(HaveKey(channelMapKey))

				// omit the timestamp as its dynamic
				g.Expect(notification.Status.States[channelMapKey]).To(Not(BeNil()))
				g.Expect(notification.Status.States[channelMapKey].Sent).To(BeTrue())
				g.Expect(notification.Status.States[channelMapKey].ErrorMessage).To(BeEquivalentTo("Successfully sent"))
			}, timeout, interval).Should(Succeed())

		})
	})

	Context("When reconciling a resource and some resources are missing", func() {

		var (
			ctx              context.Context
			notificationName string
			templateName     string
			channelName      string
			notif            *notificationv1.Notification
			tmpl             *notificationv1.NotificationTemplate
		)

		BeforeEach(func() {
			ctx = context.Background()

			notificationName = randName("notif")
			templateName = randName(notificationPurpose + "--mail")
			channelName = randName("eni--hyperion--mail")

		})

		AfterEach(func() {
			By("Cleanup the specific resource instance Notification")
			deleteIfExists(ctx, notif)

			By("Cleanup the specific resource instance Notification template")
			deleteIfExists(ctx, tmpl)
		})

		It("should fail to reconcile the notification if a channel is missing", func() {
			By("creating the custom resource for the Kind Notification Template")
			tmpl = newTestNotificationTemplate(ctx, templateName, defaultNamespace)

			By("creating the custom resource for the Kind Notification")
			notif = newTestNotification(ctx, notificationName, defaultNamespace, withChannel(channelName, defaultNamespace))

			By("not creating the custom resource for the Kind Notification channel")

			By("Updating the status.states with a failed SendState")

			Eventually(func(g Gomega) {
				notification := &notificationv1.Notification{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notif.Name,
					Namespace: defaultNamespace,
				}, notification)
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

				g.Expect(notification.Status.Conditions).To(ConsistOf(
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Type":    Equal("Ready"),
						"Reason":  Equal("NotificationSendingFailed"),
						"Message": Equal("Some notifications were not sent"),
						"Status":  Equal(metav1.ConditionStatus("False")),
					}),
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Type":    Equal("Processing"),
						"Reason":  Equal("Blocked"),
						"Message": Equal("Channel or template cannot be resolved"),
						"Status":  Equal(metav1.ConditionStatus("False")),
					}),
				))

				// notifications are sent
				g.Expect(notification.Status.States).To(HaveLen(1))

				// omit the timestamp as its dynamic
				channelMapKey := defaultNamespace + "/" + channelName
				g.Expect(notification.Status.States).To(HaveKey(channelMapKey))

				errorMessage := fmt.Sprintf("Error getting channel \"%v\": failed to get object: NotificationChannel.notification.cp.ei.telekom.de \"%v\" not found", channelMapKey, channelName)
				g.Expect(notification.Status.States[channelMapKey]).To(Not(BeNil()))
				g.Expect(notification.Status.States[channelMapKey].Sent).To(BeFalse())
				g.Expect(notification.Status.States[channelMapKey].ErrorMessage).To(BeEquivalentTo(errorMessage))
			}, timeout, interval).Should(Succeed())

		})
	})

	Context("Index for template works correctly", func() {
		var (
			ctx                      context.Context
			templateName             string
			channelName              string
			matchingNotificationName string
			otherNotificationName    string
			purpose1Name             string
			purpose2Name             string

			matchingNotification *notificationv1.Notification
			otherNotification    *notificationv1.Notification
			template             *notificationv1.NotificationTemplate
		)

		BeforeEach(func() {
			ctx = context.Background()

			// the index is parsing the channel name to resolve the template value, so this is not randomized
			By("Creating a channel name")
			channelName = "eni--hyperion--mail"

			By("Creating a matching notification")
			purpose1Name = randName("matching-purpose")
			matchingNotificationName = randName("matching-notification")
			matchingNotification = newTestNotification(ctx, matchingNotificationName, defaultNamespace,
				withChannel(channelName, defaultNamespace),
				withPurpose(purpose1Name),
			)

			By("Creating an other notification")
			purpose2Name = randName("other-purpose")
			otherNotificationName = randName("other-notification")
			otherNotification = newTestNotification(ctx, otherNotificationName, defaultNamespace,
				withChannel(channelName, defaultNamespace),
				withPurpose(purpose2Name),
			)

			By("Creating a template")
			templateName = purpose1Name + "--mail"
			template = newTestNotificationTemplate(ctx, templateName, testEnvironment)
		})

		AfterEach(func() {
			By("Cleanup the matching Notification")
			deleteIfExists(ctx, matchingNotification)
			By("Cleanup the other Notification")
			deleteIfExists(ctx, otherNotification)
			By("Cleanup the template")
			deleteIfExists(ctx, template)
		})

		It("should return only notifications matching the indexed template", func() {
			// Eventually to account for cache sync
			var reqs []ctrl.Request
			Eventually(func() int {
				reqs = notificationReconciler.MapTemplateToNotification(context.Background(), template)
				return len(reqs)
			}, timeout, interval).Should(Equal(1))

			Expect(reqs[0].NamespacedName.Name).To(Equal(matchingNotificationName))
		})
	})

	Context("Index for channels works correctly", func() {
		var (
			ctx                      context.Context
			channelName              string
			otherChannelName         string
			matchingNotificationName string
			otherNotificationName    string
			purpose1Name             string
			purpose2Name             string

			matchingNotification *notificationv1.Notification
			otherNotification    *notificationv1.Notification
			channel              *notificationv1.NotificationChannel
		)

		BeforeEach(func() {
			ctx = context.Background()

			// the index is parsing the channel name to resolve the template value, so this is not randomized
			By("Creating a channel name")
			channelName = randName("eni--hyperion--mail")

			By("Creating a matching notification")
			purpose1Name = randName("matching-purpose")
			matchingNotificationName = randName("matching-notification")
			matchingNotification = newTestNotification(ctx, matchingNotificationName, defaultNamespace,
				withChannel(channelName, defaultNamespace),
				withPurpose(purpose1Name),
			)

			By("Creating an other notification - different channel")
			otherChannelName = randName("eni--pandora--mail")
			purpose2Name = randName("other-purpose")
			otherNotificationName = randName("other-notification")
			otherNotification = newTestNotification(ctx, otherNotificationName, defaultNamespace,
				withChannel(otherChannelName, defaultNamespace),
				withPurpose(purpose2Name),
			)

			By("Creating a channel")
			channel = newTestNotificationChannel(ctx, channelName, defaultNamespace)
		})

		AfterEach(func() {
			By("Cleanup the matching Notification")
			deleteIfExists(ctx, matchingNotification)
			By("Cleanup the other Notification")
			deleteIfExists(ctx, otherNotification)
			By("Cleanup the channel")
			deleteIfExists(ctx, channel)
		})

		It("should return only notifications matching the indexed channel", func() {
			// Eventually to account for cache sync
			var reqs []ctrl.Request
			Eventually(func() int {
				reqs = notificationReconciler.MapChannelToNotification(context.Background(), channel)
				return len(reqs)
			}, timeout, interval).Should(Equal(1))

			Expect(reqs[0].NamespacedName.Name).To(Equal(matchingNotificationName))
		})
	})

	Context("Notifications without channels", func() {
		var (
			ctx                  context.Context
			templateName         string
			channelName          string
			otherChannelName     string
			notificationName     string
			otherNotificatioName string
			purposeName          string

			notification      *notificationv1.Notification
			otherNotification *notificationv1.Notification
			template          *notificationv1.NotificationTemplate
			channel           *notificationv1.NotificationChannel
			otherChannel      *notificationv1.NotificationChannel
		)

		BeforeEach(func() {
			ctx = context.Background()
			By("creating a new purpose name")
			purposeName = randName("purpose")

			By("creating the custom resource for the Kind Notification Template")
			// tie this to the notification purpose, as we use template name resolution via convention
			// purpose-12351--mail is valid
			// purpose--mail-12351 is not valid
			templateName = purposeName + "--mail"
			template = newTestNotificationTemplate(ctx, templateName, testEnvironment)

			By("creating the custom resource for the Kind Notification Channel")
			channelName = randName("eni--hyperion--mail")
			channel = newTestNotificationChannel(ctx, channelName, defaultNamespace)

			By("creating the custom resource for the Kind Notification without channels")
			notificationName = randName("notification")
			notification = newTestNotification(ctx, notificationName, defaultNamespace, withPurpose(purposeName))
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance Notification")
			deleteIfExists(ctx, notification)

			By("Cleanup the specific resource instance Notification channel")
			deleteIfExists(ctx, channel)

			By("Cleanup the specific resource instance Notification template")
			deleteIfExists(ctx, template)
		})

		It("can resolve channels from notifications namespace", func() {
			By("Checking if status contains states for all channels")
			Eventually(func(g Gomega) {
				notification := &notificationv1.Notification{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notificationName,
					Namespace: defaultNamespace,
				}, notification)
				g.Expect(err).To(BeNil())

				g.Expect(notification.Spec.Purpose).To(Equal(purposeName))
				g.Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeUser))
				g.Expect(notification.Spec.Sender.Name).To(Equal("John Snow"))

				// to be able to compare jsons with different order of fields
				ExpectJSONEqual(notification.Spec.Properties.Raw, []byte(`{"subjectValue":"awesomeSubject", "bodyValue":"awesomeBody"}`))

				g.Expect(notification.Status.Conditions).To(HaveLen(2))
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

				// notifications are sent
				g.Expect(notification.Status.States).To(HaveLen(1))

				channelMapKey := defaultNamespace + "/" + channelName
				g.Expect(notification.Status.States).To(HaveKey(channelMapKey))

				// omit the timestamp as its dynamic
				g.Expect(notification.Status.States[channelMapKey]).To(Not(BeNil()))
				g.Expect(notification.Status.States[channelMapKey].Sent).To(BeTrue())
				g.Expect(notification.Status.States[channelMapKey].ErrorMessage).To(BeEquivalentTo("Successfully sent"))
			}, timeout, interval).Should(Succeed())
		})

		It("doesnt impact notifications with channels set", func() {
			By("Creating the channels - 2nd in the namespace")
			otherChannelName = randName("other-purpose")
			otherChannel = newTestNotificationChannel(ctx, otherChannelName, defaultNamespace)

			By("Creating the notification with 1 channel explicitly listed")
			otherNotificatioName = randName("other-notification")
			otherNotification = newTestNotification(ctx, otherNotificatioName, defaultNamespace, withChannel(otherChannelName, defaultNamespace), withPurpose(purposeName))

			By("Checking if status contains states only for the explicitly listen channel")
			Eventually(func(g Gomega) {
				notification := &notificationv1.Notification{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      otherNotificatioName,
					Namespace: defaultNamespace,
				}, notification)
				g.Expect(err).To(BeNil())

				g.Expect(notification.Spec.Purpose).To(Equal(purposeName))
				g.Expect(notification.Spec.Sender.Type).To(Equal(notificationv1.SenderTypeUser))
				g.Expect(notification.Spec.Sender.Name).To(Equal("John Snow"))
				g.Expect(notification.Spec.Channels).To(HaveLen(1))

				// to be able to compare jsons with different order of fields
				ExpectJSONEqual(notification.Spec.Properties.Raw, []byte(`{"subjectValue":"awesomeSubject", "bodyValue":"awesomeBody"}`))

				g.Expect(notification.Status.Conditions).To(HaveLen(2))
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

				// notifications are sent
				g.Expect(notification.Status.States).To(HaveLen(1))

				channelMapKey := defaultNamespace + "/" + otherChannelName
				g.Expect(notification.Status.States).To(HaveKey(channelMapKey))

				// omit the timestamp as its dynamic
				g.Expect(notification.Status.States[channelMapKey]).To(Not(BeNil()))
				g.Expect(notification.Status.States[channelMapKey].Sent).To(BeTrue())
				g.Expect(notification.Status.States[channelMapKey].ErrorMessage).To(BeEquivalentTo("Successfully sent"))
			}, timeout, interval).Should(Succeed())

			// cleanup
			By("Cleanup the specific resource instance Notification channel - the other instance")
			deleteIfExists(ctx, otherChannel)

			By("Cleanup the specific resource instance Notification - the other instance")
			deleteIfExists(ctx, otherNotification)
		})
	})
})

func ExpectJSONEqual(actualJSON, expectedJSON []byte) {
	var actual, expected map[string]interface{}
	Expect(json.Unmarshal(actualJSON, &actual)).To(Succeed())
	Expect(json.Unmarshal(expectedJSON, &expected)).To(Succeed())
	Expect(actual).To(Equal(expected))
}

type NotificationOption = func(notification *notificationv1.Notification)

func newTestNotificationTemplate(ctx context.Context, name, namespace string) *notificationv1.NotificationTemplate {
	template := &notificationv1.NotificationTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
	Expect(k8sClient.Create(ctx, template)).To(Succeed())
	return template
}

func newTestNotificationChannel(ctx context.Context, name, namespace string) *notificationv1.NotificationChannel {
	fromString := "test.from@somewhere.test"
	channel := &notificationv1.NotificationChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
	Expect(k8sClient.Create(ctx, channel)).To(Succeed())
	return channel
}

func newTestNotification(ctx context.Context, name, namespace string, opts ...NotificationOption) *notificationv1.Notification {
	notification := &notificationv1.Notification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
				//{
				//	Name:      channelName,
				//	Namespace: "default",
				//},
			},
			Properties: runtime.RawExtension{
				Raw: []byte(`{"subjectValue":"awesomeSubject", "bodyValue":"awesomeBody"}`),
			},
		},
	}

	for _, opt := range opts {
		opt(notification)
	}

	Expect(k8sClient.Create(ctx, notification)).To(Succeed())
	return notification
}

func withChannel(name string, namespace string) NotificationOption {
	return func(notification *notificationv1.Notification) {
		if notification.Spec.Channels == nil {
			notification.Spec.Channels = []commontypes.ObjectRef{}
		}
		notification.Spec.Channels = append(notification.Spec.Channels, commontypes.ObjectRef{
			Name:      name,
			Namespace: namespace,
		})
	}
}

func withPurpose(purpose string) NotificationOption {
	return func(notification *notificationv1.Notification) {
		notification.Spec.Purpose = purpose
	}
}

func randName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, GinkgoParallelProcess()*1000+rand.Int())
}

func deleteIfExists(ctx context.Context, obj client.Object) {
	err := k8sClient.Delete(ctx, obj)
	if err != nil && !errors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}
