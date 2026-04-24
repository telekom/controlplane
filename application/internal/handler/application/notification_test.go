// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/reminder"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

func setupNotificationMocks(mockClient *fake.MockJanitorClient) {
	scheme := newScheme()
	mockClient.EXPECT().Scheme().Return(scheme).Maybe()

	// List notification channels
	mockClient.EXPECT().
		List(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()

	// CreateOrUpdate for notification
	mockClient.EXPECT().
		CreateOrUpdate(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ pkgclient.Object, fn controllerutil.MutateFn) {
			if fn != nil {
				_ = fn()
			}
		}).
		Return(controllerutil.OperationResultCreated, nil).Maybe()
}

var _ = Describe("Notification Helpers", func() {
	var (
		ctx context.Context
		app *applicationv1.Application
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, "test-env")
		app = &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "test-ns",
				UID:       "test-uid",
			},
			Spec: applicationv1.ApplicationSpec{
				Team:      "test-team",
				TeamEmail: "team@example.com",
				Secret:    "$<ref:secret>",
				Zone: commontypes.ObjectRef{
					Name:      "test-zone",
					Namespace: "test-ns",
				},
				NeedsClient: true,
			},
		}
	})

	Describe("sendRotationCompletedNotification", func() {
		It("should send a notification successfully", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			ctx = client.WithClient(ctx, mockClient)
			setupNotificationMocks(mockClient)

			rotatedExpires := metav1.NewTime(time.Now().Add(24 * time.Hour))
			currentExpires := metav1.NewTime(time.Now().Add(48 * time.Hour))
			app.Status.RotatedExpiresAt = &rotatedExpires
			app.Status.CurrentExpiresAt = &currentExpires

			ref, err := sendRotationCompletedNotification(ctx, app)
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).ToNot(BeNil())
		})

		It("should send a notification without expiry timestamps", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			ctx = client.WithClient(ctx, mockClient)
			setupNotificationMocks(mockClient)

			ref, err := sendRotationCompletedNotification(ctx, app)
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).ToNot(BeNil())
		})
	})

	Describe("sendSecretExpiringNotifications", func() {
		var zone *adminv1.Zone

		BeforeEach(func() {
			zone = &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-zone",
					Namespace: "test-ns",
				},
				Spec: adminv1.ZoneSpec{
					IdentityProvider: adminv1.IdentityProviderConfig{
						SecretRotation: &adminv1.SecretRotationConfig{
							Enabled:          true,
							ExpirationPeriod: metav1.Duration{Duration: 30 * 24 * time.Hour},
							GracePeriod:      metav1.Duration{Duration: 24 * time.Hour},
							NotificationThresholds: []reminder.Threshold{
								{Before: metav1.Duration{Duration: 7 * 24 * time.Hour}},
							},
						},
					},
				},
			}
		})

		It("should return nil when CurrentExpiresAt is nil", func() {
			app.Status.CurrentExpiresAt = nil
			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(BeEmpty())
		})

		It("should return nil when secret rotation is not configured", func() {
			zone.Spec.IdentityProvider.SecretRotation = nil
			expiresAt := metav1.NewTime(time.Now().Add(1 * time.Hour))
			app.Status.CurrentExpiresAt = &expiresAt

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(BeEmpty())
		})

		It("should return nil when secret rotation is disabled", func() {
			zone.Spec.IdentityProvider.SecretRotation.Enabled = false
			expiresAt := metav1.NewTime(time.Now().Add(1 * time.Hour))
			app.Status.CurrentExpiresAt = &expiresAt

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(BeEmpty())
		})

		It("should not send when expiry is far in the future (beyond threshold)", func() {
			expiresAt := metav1.NewTime(time.Now().Add(20 * 24 * time.Hour)) // 20 days out, threshold is 7 days
			app.Status.CurrentExpiresAt = &expiresAt

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(BeEmpty())
		})

		It("should not send when secret has already expired", func() {
			expiresAt := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			app.Status.CurrentExpiresAt = &expiresAt

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(BeEmpty())
		})

		It("should send notification when within the threshold", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			ctx = client.WithClient(ctx, mockClient)
			setupNotificationMocks(mockClient)

			expiresAt := metav1.NewTime(time.Now().Add(3 * 24 * time.Hour)) // 3 days out, threshold is 7 days
			app.Status.CurrentExpiresAt = &expiresAt

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(HaveLen(1))
			Expect(app.Status.SentNotifications[0].Threshold).To(Equal((7 * 24 * time.Hour).String()))
		})

		It("should send notification when expiry is just inside the threshold boundary", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			ctx = client.WithClient(ctx, mockClient)
			setupNotificationMocks(mockClient)

			expiresAt := metav1.NewTime(time.Now().Add(7*24*time.Hour - time.Minute))
			app.Status.CurrentExpiresAt = &expiresAt

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(HaveLen(1))
		})

		It("should not send duplicate one-shot notification", func() {
			expiresAt := metav1.NewTime(time.Now().Add(3 * 24 * time.Hour))
			app.Status.CurrentExpiresAt = &expiresAt

			// Mark as already sent
			app.Status.SentNotifications = []reminder.SentReminder{{
				Threshold: (7 * 24 * time.Hour).String(),
				Ref:       commontypes.ObjectRef{Name: "prev", Namespace: "test-ns"},
				SentAt:    metav1.NewTime(time.Now().Add(-1 * time.Hour)),
			}}

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			// Should still have exactly 1 entry (not updated since it's one-shot)
			Expect(app.Status.SentNotifications).To(HaveLen(1))
			Expect(app.Status.SentNotifications[0].Ref.Name).To(Equal("prev"))
		})

		It("should only fire tightest threshold when multiple are in window", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			ctx = client.WithClient(ctx, mockClient)
			setupNotificationMocks(mockClient)

			zone.Spec.IdentityProvider.SecretRotation.NotificationThresholds = []reminder.Threshold{
				{Before: metav1.Duration{Duration: 30 * 24 * time.Hour}}, // 30d
				{Before: metav1.Duration{Duration: 7 * 24 * time.Hour}},  // 7d
				{Before: metav1.Duration{Duration: 24 * time.Hour}},      // 1d
			}

			expiresAt := metav1.NewTime(time.Now().Add(3 * 24 * time.Hour)) // 3 days out
			app.Status.CurrentExpiresAt = &expiresAt

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			// Only the 7d threshold should fire (tightest in window), not 30d
			Expect(app.Status.SentNotifications).To(HaveLen(1))
			Expect(app.Status.SentNotifications[0].Threshold).To(Equal((7 * 24 * time.Hour).String()))
		})

		It("should repeat notification when repeat interval has elapsed", func() {
			mockClient := fake.NewMockJanitorClient(GinkgoT())
			ctx = client.WithClient(ctx, mockClient)
			setupNotificationMocks(mockClient)

			repeatInterval := metav1.Duration{Duration: 24 * time.Hour}
			zone.Spec.IdentityProvider.SecretRotation.NotificationThresholds = []reminder.Threshold{
				{Before: metav1.Duration{Duration: 7 * 24 * time.Hour}, Repeat: &repeatInterval},
			}

			expiresAt := metav1.NewTime(time.Now().Add(3 * 24 * time.Hour))
			app.Status.CurrentExpiresAt = &expiresAt

			// Last sent 25 hours ago (> 24h repeat interval)
			app.Status.SentNotifications = []reminder.SentReminder{{
				Threshold: (7 * 24 * time.Hour).String(),
				Ref:       commontypes.ObjectRef{Name: "prev", Namespace: "test-ns"},
				SentAt:    metav1.NewTime(time.Now().Add(-25 * time.Hour)),
			}}

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			Expect(app.Status.SentNotifications).To(HaveLen(1))
			// Ref should be updated (upserted)
			Expect(app.Status.SentNotifications[0].Ref.Name).ToNot(Equal("prev"))
		})

		It("should not repeat notification when repeat interval has not elapsed", func() {
			repeatInterval := metav1.Duration{Duration: 24 * time.Hour}
			zone.Spec.IdentityProvider.SecretRotation.NotificationThresholds = []reminder.Threshold{
				{Before: metav1.Duration{Duration: 7 * 24 * time.Hour}, Repeat: &repeatInterval},
			}

			expiresAt := metav1.NewTime(time.Now().Add(3 * 24 * time.Hour))
			app.Status.CurrentExpiresAt = &expiresAt

			// Last sent 12 hours ago (< 24h repeat interval)
			app.Status.SentNotifications = []reminder.SentReminder{{
				Threshold: (7 * 24 * time.Hour).String(),
				Ref:       commontypes.ObjectRef{Name: "prev", Namespace: "test-ns"},
				SentAt:    metav1.NewTime(time.Now().Add(-12 * time.Hour)),
			}}

			err := sendSecretExpiringNotifications(ctx, app, zone)
			Expect(err).ToNot(HaveOccurred())
			// Should not have been updated
			Expect(app.Status.SentNotifications[0].Ref.Name).To(Equal("prev"))
		})
	})
})
