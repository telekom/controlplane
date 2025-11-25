// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	notificationsconfig "github.com/telekom/controlplane/notification/internal/config"
	handlers "github.com/telekom/controlplane/notification/internal/handler"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"time"
)

var _ = Describe("Notification Handler", func() {

	Context("Reconciling a partially processed resource", func() {

		const notificationPurpose = "test-purpose"

		const notificationName = "test-notification"
		const channel2Name = "eni--hyperion--chat"

		SendingTime := metav1.NewTime(time.Date(1989, time.May, 7, 0, 0, 0, 0, time.UTC))

		var k8sScheme *runtime.Scheme
		var ctx context.Context
		var fakeClient *fakeclient.MockJanitorClient

		BeforeEach(func() {
			ctx = context.Background()
			fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
			ctx = cclient.WithClient(ctx, fakeClient)

			k8sScheme = runtime.NewScheme()
			err := notificationv1.AddToScheme(k8sScheme)
			Expect(err).NotTo(HaveOccurred())
			err = test.AddToScheme(k8sScheme)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not try to send notifications that were already successfully sent", func() {
			By("creating a notification handler")
			notificationHandler := &handlers.NotificationHandler{}

			By("creating a new notification with partially successful send states")
			notification := &notificationv1.Notification{
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
							Name:      channel2Name,
							Namespace: "default",
						},
					},
					Properties: runtime.RawExtension{
						Raw: []byte(`{"subjectValue":"awesomeSubject", "bodyValue":"awesomeBody"}`),
					},
				},
				Status: notificationv1.NotificationStatus{
					States: map[string]notificationv1.SendState{
						"default/" + channel2Name: {
							Timestamp:    SendingTime,
							Sent:         true,
							ErrorMessage: "Successfully sent",
						},
					},
				},
			}

			By("calling the handlers createOrUpdate func with the notification")
			err := notificationHandler.CreateOrUpdate(ctx, notification)

			By("expecting the createOrUpdate func runs without error")
			Expect(err).ToNot(HaveOccurred())

			By("expecting the notification CR to be successfully processed")
			Expect(notification.Status.Conditions).To(HaveLen(2))
			Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
			Expect(meta.IsStatusConditionTrue(notification.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

			By("not modifying the timestamp of the sent notification")
			Expect(notification.Status.States).To(HaveKey("default/eni--hyperion--chat"))
			Expect(notification.Status.States["default/eni--hyperion--chat"]).To(Not(BeNil()))
			Expect(notification.Status.States["default/eni--hyperion--chat"].Sent).To(BeTrue())
			Expect(notification.Status.States["default/eni--hyperion--chat"].ErrorMessage).To(BeEquivalentTo("Successfully sent"))
			Expect(notification.Status.States["default/eni--hyperion--chat"].Timestamp.Unix()).To(BeEquivalentTo(SendingTime.Unix()))
		})

		It("should delete notifications eligible for housekeeping", func() {
			By("creating the housekeeping configuration with 0 ttl - immediate removal")
			housekeepingConfig := notificationsconfig.NotificationHousekeepingConfig{
				TTLMonthsAfterFinished: 0,
			}

			By("creating a notification handler with housekeeping configuration")
			notificationHandler := &handlers.NotificationHandler{
				HousekeepingConfig: housekeepingConfig,
			}

			By("creating a new notification with successfully sent notifications and ready state")
			rdyCondition := condition.NewReadyCondition("Ready", "All channels sent")
			rdyCondition.LastTransitionTime = metav1.NewTime(SendingTime.Add(1 * time.Second))

			notification := &notificationv1.Notification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notificationName + "2",
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
							Name:      channel2Name,
							Namespace: "default",
						},
					},
					Properties: runtime.RawExtension{
						Raw: []byte(`{"subjectValue":"awesomeSubject", "bodyValue":"awesomeBody"}`),
					},
				},
				Status: notificationv1.NotificationStatus{
					Conditions: []metav1.Condition{
						rdyCondition,
					},
					States: map[string]notificationv1.SendState{
						"default/" + channel2Name: {
							Timestamp:    SendingTime,
							Sent:         true,
							ErrorMessage: "Successfully sent",
						},
					},
				},
			}

			By("expecting the client will be called to delete the notification")
			fakeClient.EXPECT().Delete(ctx, notification).Return(nil)

			By("calling the handlers createOrUpdate func with the notification")
			err := notificationHandler.CreateOrUpdate(ctx, notification)

			By("expecting the createOrUpdate func runs without error")
			Expect(err).ToNot(HaveOccurred())

		})
	})
})
