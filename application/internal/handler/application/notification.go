// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	admin "github.com/telekom/controlplane/admin/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
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

// sendSecretExpiringNotification sends a notification urging the owning team to
// rotate their secret before it expires. It only sends when the time until expiry
// is within the configured NotificationThreshold on the zone.
func sendSecretExpiringNotification(ctx context.Context, app *application.Application, zone *admin.Zone) (*types.ObjectRef, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Evaluating secret-expiring notification",
		"application", app.Name,
		"currentExpiresAt", app.Status.CurrentExpiresAt,
	)

	if app.Status.CurrentExpiresAt == nil {
		log.V(1).Info("Skipping secret-expiring notification: currentExpiresAt is nil", "application", app.Name)
		return nil, nil
	}

	if zone.Spec.IdentityProvider.SecretRotation == nil || !zone.Spec.IdentityProvider.SecretRotation.Enabled {
		log.V(1).Info("Skipping secret-expiring notification: secret rotation not enabled on zone", "application", app.Name, "zone", zone.Name)
		return nil, nil
	}

	threshold := zone.Spec.IdentityProvider.SecretRotation.NotificationThreshold.Duration
	timeUntilExpiry := time.Until(app.Status.CurrentExpiresAt.Time)

	// Only send if within the notification threshold and not already expired
	if timeUntilExpiry > threshold || timeUntilExpiry <= 0 {
		log.V(1).Info("Skipping secret-expiring notification: outside threshold window",
			"application", app.Name,
			"timeUntilExpiry", timeUntilExpiry,
			"threshold", threshold,
		)
		return nil, nil
	}

	env := contextutil.EnvFromContextOrDie(ctx)

	properties := map[string]any{
		"application":      app.Name,
		"team":             app.Spec.Team,
		"teamEmail":        app.Spec.TeamEmail,
		"environment":      env,
		"currentExpiresAt": app.Status.CurrentExpiresAt.Format(time.RFC3339),
	}

	notification, err := builder.New().
		WithNamespace(app.Namespace).
		WithOwner(app).
		WithSender(notificationv1.SenderTypeSystem, "ApplicationService").
		WithPurpose(PurposeSecretExpiring).
		WithName(PurposeSecretExpiring).
		WithDefaultChannels(ctx, app.Namespace).
		WithProperties(properties).
		Send(ctx)
	if err != nil {
		return nil, err
	}

	return types.ObjectRefFromObject(notification), nil
}
