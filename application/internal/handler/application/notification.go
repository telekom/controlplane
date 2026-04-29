// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	admin "github.com/telekom/controlplane/admin/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
)

const (
	// PurposeRotationCompleted is the notification purpose for a completed secret rotation.
	PurposeRotationCompleted = "secret-rotation-completed"
	// PurposeSecretExpiring is the notification purpose for a secret that is about to expire.
	PurposeSecretExpiring = "secret-rotation-expiring"
)

// sendRotationCompletedNotification sends a notification informing the owning team
// that their application's secret has been successfully rotated and that the old
// secret remains valid for the grace period.
func sendRotationCompletedNotification(ctx context.Context, app *application.Application) (*types.ObjectRef, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Sending rotation-completed notification",
		"application", app.Name,
		"team", app.Spec.Team,
		"rotatedExpiresAt", app.Status.RotatedExpiresAt,
		"currentExpiresAt", app.Status.CurrentExpiresAt,
	)

	env := contextutil.EnvFromContextOrDie(ctx)

	properties := map[string]any{
		"application": app.Name,
		"team":        app.Spec.Team,
		"teamEmail":   app.Spec.TeamEmail,
		"environment": env,
	}

	if app.Status.RotatedExpiresAt != nil {
		properties["rotatedExpiresAt"] = app.Status.RotatedExpiresAt.Format(time.RFC3339)
	}
	if app.Status.CurrentExpiresAt != nil {
		properties["currentExpiresAt"] = app.Status.CurrentExpiresAt.Format(time.RFC3339)
	}

	notification, err := builder.New().
		WithNamespace(app.Namespace).
		WithOwner(app).
		WithSender(notificationv1.SenderTypeSystem, "ApplicationService").
		WithPurpose(PurposeRotationCompleted).
		WithName(PurposeRotationCompleted).
		WithDefaultChannels(ctx, app.Namespace).
		WithProperties(properties).
		Send(ctx)
	if err != nil {
		return nil, err
	}

	return types.ObjectRefFromObject(notification), nil
}

// sendSecretExpiringNotifications evaluates the configured notification thresholds
// against the application's secret expiry and sends at most one reminder notification
// per reconciliation cycle (the tightest matching threshold only).
//
// It mutates app.Status.SentNotifications to track what was sent.
func sendSecretExpiringNotifications(ctx context.Context, app *application.Application, zone *admin.Zone) error {
	log := logr.FromContextOrDiscard(ctx).WithName("secret-expiring-notification")

	if app.Status.CurrentExpiresAt == nil {
		log.V(1).Info("Skipping secret-expiring notifications: currentExpiresAt is nil", "application", app.Name)
		return nil
	}

	rotCfg := zone.Spec.IdentityProvider.SecretRotation
	if rotCfg == nil || !rotCfg.Enabled || len(rotCfg.NotificationThresholds) == 0 {
		log.V(1).Info("Skipping secret-expiring notifications: not configured", "application", app.Name)
		return nil
	}

	now := time.Now()
	deadline := app.Status.CurrentExpiresAt.Time

	log.V(1).Info("Evaluating secret-expiring notifications",
		"application", app.Name,
		"deadline", deadline.Format(time.RFC3339),
		"now", now.Format(time.RFC3339),
		"timeUntilDeadline", deadline.Sub(now),
		"numThresholds", len(rotCfg.NotificationThresholds),
		"numSentNotifications", len(app.Status.SentNotifications),
	)

	pending := reminder.Evaluate(deadline, rotCfg.NotificationThresholds, app.Status.SentNotifications, now)
	if len(pending) == 0 {
		log.V(1).Info("No pending secret-expiring notifications", "application", app.Name)
		return nil
	}

	// Evaluate returns at most one pending reminder (tightest threshold).
	p := pending[0]
	log.V(1).Info("Sending secret-expiring notification",
		"application", app.Name,
		"threshold", p.Key,
	)

	ref, err := sendSingleExpiringNotification(ctx, app, p.Key, now)
	if err != nil {
		return err
	}

	app.Status.SentNotifications = reminder.UpsertSent(app.Status.SentNotifications, &reminder.SentReminder{
		Threshold: p.Key,
		Ref:       *ref,
		SentAt:    metav1.NewTime(now),
	})

	log.V(1).Info("Successfully sent secret-expiring notification", "ref", ref, "threshold", p.Key)
	return nil
}

// sendSingleExpiringNotification sends one secret-expiring notification for a specific threshold.
func sendSingleExpiringNotification(ctx context.Context, app *application.Application, thresholdKey string, now time.Time) (*types.ObjectRef, error) {
	env := contextutil.EnvFromContextOrDie(ctx)

	name := fmt.Sprintf("%s-%s", PurposeSecretExpiring, thresholdKey)

	deadline := app.Status.CurrentExpiresAt.Time
	timeUntilExpiry := deadline.Sub(now).Round(time.Minute)

	properties := map[string]any{
		"application":      app.Name,
		"team":             app.Spec.Team,
		"teamEmail":        app.Spec.TeamEmail,
		"environment":      env,
		"currentExpiresAt": app.Status.CurrentExpiresAt.Format(time.RFC3339),
		"thresholdBefore":  thresholdKey,
		"timeUntilExpiry":  formatTimeUntilExpiry(timeUntilExpiry),
		"sentAt":           now.Format(time.RFC3339),
	}

	notification, err := builder.New().
		WithNamespace(app.Namespace).
		WithOwner(app).
		WithSender(notificationv1.SenderTypeSystem, "ApplicationService").
		WithPurpose(PurposeSecretExpiring).
		WithName(name).
		WithDefaultChannels(ctx, app.Namespace).
		WithProperties(properties).
		Send(ctx)
	if err != nil {
		return nil, err
	}

	return types.ObjectRefFromObject(notification), nil
}

func formatTimeUntilExpiry(d time.Duration) string {
	if d < 0 {
		return "expired"
	}
	return d.String()
}
