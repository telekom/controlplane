// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"time"

	admin "github.com/telekom/controlplane/admin/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
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
func sendRotationCompletedNotification(ctx context.Context, app *application.Application) error {
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

	_, err := builder.New().
		WithNamespace(app.Namespace).
		WithOwner(app).
		WithSender(notificationv1.SenderTypeSystem, "ApplicationService").
		WithPurpose(PurposeRotationCompleted).
		WithName(PurposeRotationCompleted).
		WithDefaultChannels(ctx, app.Namespace).
		WithProperties(properties).
		Send(ctx)

	return err
}

// sendSecretExpiringNotification sends a notification urging the owning team to
// rotate their secret before it expires. It only sends when the time until expiry
// is within the configured NotificationThreshold on the zone.
func sendSecretExpiringNotification(ctx context.Context, app *application.Application, zone *admin.Zone) error {
	if app.Status.CurrentExpiresAt == nil {
		return nil
	}

	if zone.Spec.IdentityProvider.SecretRotation == nil || !zone.Spec.IdentityProvider.SecretRotation.Enabled {
		return nil
	}

	threshold := zone.Spec.IdentityProvider.SecretRotation.NotificationThreshold.Duration
	timeUntilExpiry := time.Until(app.Status.CurrentExpiresAt.Time)

	// Only send if within the notification threshold and not already expired
	if timeUntilExpiry > threshold || timeUntilExpiry <= 0 {
		return nil
	}

	env := contextutil.EnvFromContextOrDie(ctx)

	properties := map[string]any{
		"application":      app.Name,
		"team":             app.Spec.Team,
		"teamEmail":        app.Spec.TeamEmail,
		"environment":      env,
		"currentExpiresAt": app.Status.CurrentExpiresAt.Format(time.RFC3339),
	}

	_, err := builder.New().
		WithNamespace(app.Namespace).
		WithOwner(app).
		WithSender(notificationv1.SenderTypeSystem, "ApplicationService").
		WithPurpose(PurposeSecretExpiring).
		WithName(PurposeSecretExpiring).
		WithDefaultChannels(ctx, app.Namespace).
		WithProperties(properties).
		Send(ctx)

	return err
}
