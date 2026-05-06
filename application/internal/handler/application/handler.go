// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package application implements the handler for the Application resource.
package application

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	admin "github.com/telekom/controlplane/admin/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/secret"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gateway "github.com/telekom/controlplane/gateway/api/v1"
	identity "github.com/telekom/controlplane/identity/api/v1"
)

var _ handler.Handler[*application.Application] = &ApplicationHandler{}

type ApplicationHandler struct{}

func (h *ApplicationHandler) CreateOrUpdate(ctx context.Context, app *application.Application) error {
	c := client.ClientFromContextOrDie(ctx)
	c.AddKnownTypeToState(&identity.Client{})
	c.AddKnownTypeToState(&gateway.Consumer{})

	app.Status.Clients = []types.ObjectRef{}
	app.Status.Consumers = []types.ObjectRef{}
	app.Status.ClientId = MakeClientName(app)

	zone, failoverZones, err := h.resolveZones(ctx, c, app)
	if err != nil {
		return err
	}

	primaryClient, err := h.ensureIdentityClients(ctx, zone, failoverZones, app)
	if err != nil {
		return err
	}

	if err := h.ensureGatewayConsumers(ctx, zone, failoverZones, app); err != nil {
		return err
	}

	if _, err := c.CleanupAll(ctx, client.OwnedBy(app)); err != nil {
		return err
	}

	rotationInProgress := h.initiateRotationIfNeeded(app)

	if c.AnyChanged() {
		app.SetCondition(
			condition.NewNotReadyCondition("SubResourceProvisioning", "At least one sub-resource has been created or updated"))
		return nil
	}

	if primaryClient != nil && !condition.IsReady(primaryClient) {
		app.SetCondition(condition.NewNotReadyCondition("SubResourceProvisioned", "Waiting for primary identity client to be ready"))
		return nil
	}

	// All sub-resources are up to date and primary client (if applicable) is ready.
	app.SetCondition(condition.NewReadyCondition("SubResourceProvisioned", "All sub-resources are up to date"))

	if app.Spec.NeedsClient {
		app.Status.ClientSecret = app.Spec.Secret
	}

	if rotationInProgress {
		completeRotation(ctx, app)
	} else if app.Spec.NeedsClient && app.Status.CurrentExpiresAt != nil {
		handleSecretExpiringNotifications(ctx, app, zone)
	}

	return nil
}

func (h *ApplicationHandler) resolveZones(ctx context.Context, c client.ScopedClient, app *application.Application) (*admin.Zone, []*admin.Zone, error) {
	zone, err := GetZone(ctx, c, app.Spec.Zone)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, nil, ctrlerrors.BlockedErrorf("Zone %s not found", app.Spec.Zone.Name)
		}
		return nil, nil, ctrlerrors.RetryableErrorf("failed to get Zone when creating application: %s", err.Error())
	}

	failoverZones := make([]*admin.Zone, 0, len(app.Spec.FailoverZones))
	if app.Spec.NeedsClient || app.Spec.NeedsConsumer {
		for _, zoneRef := range app.Spec.FailoverZones {
			foZone, err := GetZone(ctx, c, zoneRef)
			if err != nil {
				if apierrors.IsNotFound(errors.Cause(err)) {
					return nil, nil, ctrlerrors.BlockedErrorf("Zone %s not found", zoneRef.Name)
				}
				return nil, nil, ctrlerrors.RetryableErrorf("failed to get Zone when creating application: %s", err.Error())
			}
			failoverZones = append(failoverZones, foZone)
		}
	}

	return zone, failoverZones, nil
}

func (h *ApplicationHandler) ensureIdentityClients(ctx context.Context, zone *admin.Zone, failoverZones []*admin.Zone, app *application.Application) (*identity.Client, error) {
	if !app.Spec.NeedsClient {
		app.Status.ClientSecret = "NOT_NEEDED"
		return nil, nil
	}

	primaryClient, err := CreateIdentityClient(ctx, zone, app)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Identity client when creating application")
	}

	for _, failoverZone := range failoverZones {
		if _, err := CreateIdentityClient(ctx, failoverZone, app, WithFailover()); err != nil {
			return nil, errors.Wrapf(err, "failed to create Identity client for failover zone %s when creating application", failoverZone.Name)
		}
	}

	return primaryClient, nil
}

func (h *ApplicationHandler) ensureGatewayConsumers(ctx context.Context, zone *admin.Zone, failoverZones []*admin.Zone, app *application.Application) error {
	if !app.Spec.NeedsConsumer {
		return nil
	}

	if err := CreateGatewayConsumer(ctx, zone, app); err != nil {
		return errors.Wrap(err, "failed to create Gateway consumer when creating application")
	}

	for _, failoverZone := range failoverZones {
		if err := CreateGatewayConsumer(ctx, failoverZone, app, WithFailover()); err != nil {
			return errors.Wrapf(err, "failed to create Gateway consumer for failover zone %s when creating application", failoverZone.Name)
		}
	}

	return nil
}

// initiateRotationIfNeeded checks if a new secret rotation should be started and returns whether rotation is in progress.
func (h *ApplicationHandler) initiateRotationIfNeeded(app *application.Application) bool {
	rotationRequested := app.Spec.RotatedSecret != ""
	rotationCond := meta.FindStatusCondition(app.Status.Conditions, secret.SecretRotationConditionType)
	rotationInProgress := rotationCond != nil && rotationCond.Reason == secret.SecretRotationReasonInProgress
	rotationAlreadyHandled := app.Spec.RotatedSecret == app.Status.RotatedClientSecret

	if rotationRequested && !rotationInProgress && !rotationAlreadyHandled {
		app.SetCondition(metav1.Condition{
			Type:    secret.SecretRotationConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  secret.SecretRotationReasonInProgress,
			Message: "Secret rotation initiated, waiting for sub-resources to converge",
		})
		return true
	}

	return rotationInProgress
}

func completeRotation(ctx context.Context, app *application.Application) {
	log := logr.FromContextOrDiscard(ctx)

	// Expiry timestamps are already propagated by CreateIdentityClient.
	// Check that they are available (identity controller has reconciled).
	if app.Status.RotatedExpiresAt == nil {
		log.Info("Secret rotation: expiry timestamps not yet available from identity client, staying InProgress",
			"application", app.Name,
		)
		return
	}

	// Set rotation status fields only after successful convergence
	app.Status.RotatedClientSecret = app.Spec.RotatedSecret

	app.SetCondition(metav1.Condition{
		Type:    secret.SecretRotationConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  secret.SecretRotationReasonSuccess,
		Message: "Secret rotation completed successfully",
	})

	// Reset expiry reminders from the previous rotation cycle
	app.Status.SentNotifications = nil

	// Send rotation-completed notification (non-blocking)
	if notificationRef, err := sendRotationCompletedNotification(ctx, app); err != nil {
		log.Error(err, "Failed to send secret-rotation-completed notification")
	} else if notificationRef != nil {
		app.Status.SentNotifications = reminder.UpsertSent(app.Status.SentNotifications, &reminder.SentReminder{
			Threshold: PurposeRotationCompleted,
			Ref:       *notificationRef,
			SentAt:    metav1.Now(),
		})
	}

	log.Info("Secret rotation completed successfully",
		"application", app.Name,
		"team", app.Spec.Team,
	)
}

func handleSecretExpiringNotifications(ctx context.Context, app *application.Application, zone *admin.Zone) {
	log := logr.FromContextOrDiscard(ctx)
	if err := sendSecretExpiringNotifications(ctx, app, zone); err != nil {
		log.Error(err, "Failed to send secret-rotation-expiring notification")
	}

	// Schedule next reconciliation for the next reminder event so we
	// wake up in time rather than waiting for the default requeue interval.
	rotCfg := zone.Spec.IdentityProvider.SecretRotation
	if rotCfg != nil && rotCfg.Enabled && len(rotCfg.NotificationThresholds) > 0 {
		now := time.Now()
		deadline := app.Status.CurrentExpiresAt.Time
		timeUntilDeadline := deadline.Sub(now)

		if nextRequeue := reminder.NextRequeue(
			deadline,
			rotCfg.NotificationThresholds,
			app.Status.SentNotifications,
			now,
		); nextRequeue > 0 {
			// Cap at timeUntilDeadline so that jitter applied by the controller
			// cannot push the next reconcile past the secret expiry.
			if timeUntilDeadline > 0 && nextRequeue > timeUntilDeadline {
				nextRequeue = timeUntilDeadline
			}
			contextutil.SetRequeueAfter(ctx, nextRequeue)
		}
	}
}

func (h *ApplicationHandler) Delete(ctx context.Context, app *application.Application) error {
	// deleted using controller reference
	return nil
}

type CreateOptions struct {
	Failover bool
}

type CreateOption func(*CreateOptions)

func WithFailover() CreateOption {
	return func(opts *CreateOptions) {
		opts.Failover = true
	}
}

func CreateIdentityClient(ctx context.Context, zone *admin.Zone, owner *application.Application, opts ...CreateOption) (*identity.Client, error) {
	options := &CreateOptions{}
	for _, opt := range opts {
		opt(options)
	}

	c := client.ClientFromContextOrDie(ctx)
	clientId := MakeClientName(owner)
	resourceName := clientId + "--" + zone.Name
	realmName := contextutil.EnvFromContextOrDie(ctx)

	// get namespace from zoneStatus
	namespace := zone.Status.Namespace

	// get Realm with realmref from namespace
	realmRef := &types.ObjectRef{
		Name:      realmName,
		Namespace: namespace,
	}

	idpClient := &identity.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: owner.GetNamespace(),
		},
	}

	mutator := func() error {
		idpClient.Labels = map[string]string{
			config.BuildLabelKey("application"): owner.Name,
			config.BuildLabelKey("team"):        owner.Spec.Team,
			config.BuildLabelKey("realm"):       realmName,
			config.BuildLabelKey("zone"):        zone.Name,
		}
		if options.Failover {
			idpClient.Labels[config.BuildLabelKey("failover")] = "true"
		}

		err := ctrl.SetControllerReference(owner, idpClient, c.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controller reference for identity client %s", resourceName)
		}

		idpClient.Spec = identity.ClientSpec{
			ClientId:     clientId,
			Realm:        realmRef,
			ClientSecret: owner.Spec.Secret,
		}

		return nil
	}

	result, err := c.CreateOrUpdate(ctx, idpClient, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Identity Client %s", resourceName)
	}

	owner.Status.Clients = append(owner.Status.Clients, *types.ObjectRefFromObject(idpClient))

	// Only propagate expiry timestamps from a converged (unchanged) primary identity client.
	// When the client was created or updated, the identity controller has not yet reconciled
	// and the status fields would be stale.
	if result == controllerutil.OperationResultNone && !options.Failover {
		owner.Status.CurrentExpiresAt = idpClient.Status.SecretExpiresAt
		if owner.Spec.RotatedSecret != "" {
			owner.Status.RotatedExpiresAt = idpClient.Status.RotatedSecretExpiresAt
		}
	}

	return idpClient, nil
}

func CreateGatewayConsumer(ctx context.Context, zone *admin.Zone, owner *application.Application, opts ...CreateOption) error {
	options := &CreateOptions{}
	for _, opt := range opts {
		opt(options)
	}

	c := client.ClientFromContextOrDie(ctx)
	clientId := MakeClientName(owner)
	resourceName := clientId + "--" + zone.Name
	realmName := contextutil.EnvFromContextOrDie(ctx)

	realmRef := types.ObjectRef{
		Name:      realmName,
		Namespace: zone.Status.Namespace,
	}

	consumer := &gateway.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: owner.GetNamespace(),
		},
	}

	mutator := func() error {
		consumer.Labels = map[string]string{
			config.BuildLabelKey("application"): owner.Name,
			config.BuildLabelKey("team"):        owner.Spec.Team,
			config.BuildLabelKey("realm"):       realmName,
			config.BuildLabelKey("zone"):        zone.Name,
		}
		if options.Failover {
			consumer.Labels[config.BuildLabelKey("failover")] = "true"
		}

		err := ctrl.SetControllerReference(owner, consumer, c.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controller reference for gateway consumer %s", resourceName)
		}
		consumer.Spec = gateway.ConsumerSpec{
			Realm: realmRef,
			Name:  clientId,
		}

		if owner.Spec.Security != nil && owner.Spec.Security.IpRestrictions != nil {
			consumer.Spec.Security = &gateway.ConsumerSecurity{
				IpRestrictions: &gateway.IpRestrictions{
					Allow: owner.Spec.Security.IpRestrictions.Allow,
					Deny:  owner.Spec.Security.IpRestrictions.Deny,
				},
			}
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, consumer, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update Gateway Consumer %s", resourceName)
	}

	owner.Status.Consumers = append(owner.Status.Consumers, *types.ObjectRefFromObject(consumer))

	return nil
}
