// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package application implements the handler for the Application resource.
package application

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
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
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
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
	app.Status.TokenUrl, err = resolveTokenURL(zone, app.Spec.Failover.Enabled)
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
			condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, "At least one sub-resource has been created or updated"))
		return nil
	}

	if primaryClient != nil && !condition.IsReady(primaryClient) {
		app.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, "Waiting for primary identity client to be ready"))
		return nil
	}

	// All sub-resources are up to date and primary client (if applicable) is ready.
	app.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "All sub-resources are up to date"))

	if app.Spec.NeedsClient {
		app.Status.ClientSecret = app.Spec.Secret
	}

	if rotationInProgress {
		completeRotation(ctx, app, primaryClient)
	} else if app.Spec.NeedsClient && app.Status.CurrentExpiresAt != nil {
		handleSecretExpiringNotifications(ctx, app, zone)
	}

	return nil
}

// resolveTokenURL returns the shared failover token URL or the default preset's token URL.
func resolveTokenURL(zone *admin.Zone, failoverEnabled bool) (string, error) {
	if failoverEnabled {
		tokenURL, err := consumerFailoverTokenURL(zone)
		if err != nil {
			return "", ctrlerrors.BlockedErrorf("zone %q does not contain a valid ConsumerFailover token URL: %s", zone.Name, err.Error())
		}
		return tokenURL, nil
	}

	preset, err := zone.Spec.GetDefaultPreset()
	if err != nil {
		return "", ctrlerrors.BlockedErrorf("zone %q does not contain the selected preset: %s", zone.Name, err.Error())
	}
	presetStatus, err := zone.Status.GetPreset(preset.Name)
	if err != nil {
		return "", ctrlerrors.BlockedErrorf("zone %q does not contain status for preset %q", zone.Name, preset.Name)
	}
	if presetStatus.Links.TokenUrl == "" {
		return "", ctrlerrors.BlockedErrorf("zone %q preset %q does not contain a token URL", zone.Name, preset.Name)
	}
	return presetStatus.Links.TokenUrl, nil
}

func consumerFailoverTokenURL(zone *admin.Zone) (string, error) {
	var tokenURL string
	// Failover provisioning aggregates consumer presets, but their shared identity endpoint must stay unambiguous.
	for i := range zone.Spec.Presets {
		preset := &zone.Spec.Presets[i]
		if !slices.Contains(consumerTrafficTypes, preset.Type) {
			continue
		}
		if !zone.Spec.PresetSupportsFeatures(preset, admin.FeatureConsumerFailover) {
			continue
		}
		status, err := zone.Status.GetPreset(preset.Name)
		if err != nil {
			return "", fmt.Errorf("status for ConsumerFailover preset %q is missing", preset.Name)
		}
		if status.Links.TokenUrl == "" {
			return "", fmt.Errorf("ConsumerFailover preset %q does not contain a token URL", preset.Name)
		}
		if tokenURL != "" && tokenURL != status.Links.TokenUrl {
			return "", fmt.Errorf("ConsumerFailover presets resolve to conflicting token URLs")
		}
		tokenURL = status.Links.TokenUrl
	}
	if tokenURL == "" {
		return "", fmt.Errorf("no enabled ConsumerFailover preset was found")
	}
	return tokenURL, nil
}

func (h *ApplicationHandler) resolveZones(ctx context.Context, c client.ScopedClient, app *application.Application) (*admin.Zone, []*admin.Zone, error) {
	logger := logr.FromContextOrDiscard(ctx)
	zone, err := GetZone(ctx, c, app.Spec.Zone)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, nil, ctrlerrors.BlockedErrorf("Zone %s not found", app.Spec.Zone.Name)
		}
		return nil, nil, ctrlerrors.RetryableErrorf("failed to get Zone when creating application: %s", err.Error())
	}

	if !app.Spec.Failover.Enabled {
		return zone, nil, nil
	}

	failoverZones, err := resolveFailoverZones(ctx, c, zone)
	if err != nil {
		return nil, nil, err
	}

	logger.Info("Resolved zones for application", "primary", zone.Name, "#failover", len(failoverZones))
	return zone, failoverZones, nil
}

// resolveFailoverZones returns non-deleting zones with at least one enabled ConsumerFailover
// preset of a consumer traffic type (API or AI). A zone whose only failover preset is Event-typed
// is not a failover target for an Application.
func resolveFailoverZones(ctx context.Context, c client.ScopedClient, primaryZone *admin.Zone) ([]*admin.Zone, error) {
	zoneList := &admin.ZoneList{}
	if err := c.List(ctx, zoneList); err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to list Zones when creating application: %s", err.Error())
	}

	// Any API or AI failover preset makes a zone eligible; applications are provisioned for all available traffic types.
	var failoverZones []*admin.Zone
	for i := range zoneList.Items {
		candidate := &zoneList.Items[i]
		if types.Equals(primaryZone, candidate) || controller.IsBeingDeleted(candidate) {
			continue
		}
		if _, err := consumerGateways(candidate, admin.FeatureConsumerFailover); err != nil {
			// The zone is well-formed but offers no failover on any consumer traffic type.
			if errors.Is(err, admin.ErrNoMatchingPreset) {
				continue
			}
			return nil, ctrlerrors.BlockedErrorf("failover zone %q is invalid: %s", candidate.Namespace+"/"+candidate.Name, err.Error())
		}
		failoverZones = append(failoverZones, candidate)
	}
	slices.SortFunc(failoverZones, func(a, b *admin.Zone) int {
		if namespaceOrder := strings.Compare(a.Namespace, b.Namespace); namespaceOrder != 0 {
			return namespaceOrder
		}
		return strings.Compare(a.Name, b.Name)
	})
	return failoverZones, nil
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

	// Primary credentials cover normal API and AI traffic; Event gateways use separate resources.
	primaryGateways, err := consumerGateways(zone)
	if err != nil {
		// BlockedErrorf formats into a plain string and does not Unwrap, so the sentinel is
		// not recoverable upstream. That is acceptable here: the controller classifies on the
		// CtrlError itself, and every blocking site in this handler reports the same way.
		return ctrlerrors.BlockedErrorf("zone %q consumer gateways cannot be resolved: %s", zone.Name, err.Error())
	}
	if err := CreateGatewayConsumers(ctx, zone, app, primaryGateways); err != nil {
		return errors.Wrap(err, "failed to create Gateway consumer when creating application")
	}

	for _, failoverZone := range failoverZones {
		// Failover credentials exist only on gateways exposed through a failover preset.
		gateways, err := consumerGateways(failoverZone, admin.FeatureConsumerFailover)
		if err != nil {
			return errors.Wrapf(err, "failed to select Gateway consumers for failover zone %s when creating application", failoverZone.Name)
		}
		if err := CreateGatewayConsumers(ctx, failoverZone, app, gateways, WithFailover()); err != nil {
			return errors.Wrapf(err, "failed to create Gateway consumer for failover zone %s when creating application", failoverZone.Name)
		}
	}

	return nil
}

// consumerTrafficTypes are the traffic types an Application provisions consumer credentials for.
// An Application carries no traffic kind, so it is provisioned for every type it could serve;
var consumerTrafficTypes = []admin.GatewayType{admin.GatewayTypeAPI, admin.GatewayTypeAI, admin.GatewayTypeEvent}

// consumerGateways returns the distinct gateways a zone exposes for the given features across
// all consumer traffic types, deduplicated by name and sorted for a stable status.
//
// Preset selection is per traffic type, but an Application needs the union: a zone offering
// API failover but no AI presets is still a valid failover target, and an AI-only failover
// gateway must still receive a consumer. So ErrNoMatchingPreset from one type is skipped and
// only reported when no type matched at all. Every other error — a malformed request
// (ErrInvalidFeatures) or a dangling gatewayRef — is returned unchanged, because those must
// block rather than silently narrow the result.
//
// The rule is written as "only ErrNoMatchingPreset may be skipped" rather than a list of
// errors that block, so any sentinel added later blocks by default. ErrAmbiguousPreset is
// therefore covered without being named, but is unreachable here: MatchingGateways is a union
// over a type, so it never calls SelectPreset or inspects defaults. A Zone violating that
// invariant can exist — it is over-provisioned here rather than blocked. Admission is the only
// thing preventing that state.
func consumerGateways(zone *admin.Zone, features ...admin.FeatureName) ([]*admin.GatewayConfig, error) {
	gatewaysByName := make(map[string]*admin.GatewayConfig)
	for _, gatewayType := range consumerTrafficTypes {
		gateways, err := zone.Spec.MatchingGateways(gatewayType, features...)
		if err != nil {
			if errors.Is(err, admin.ErrNoMatchingPreset) {
				continue
			}
			return nil, err
		}
		for _, gateway := range gateways {
			gatewaysByName[gateway.Name] = gateway
		}
	}

	if len(gatewaysByName) == 0 {
		return nil, fmt.Errorf("%w: no preset of type %v provides features %v",
			admin.ErrNoMatchingPreset, consumerTrafficTypes, features)
	}

	gateways := slices.Collect(maps.Values(gatewaysByName))
	slices.SortFunc(gateways, func(a, b *admin.GatewayConfig) int { return strings.Compare(a.Name, b.Name) })
	return gateways, nil
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

func completeRotation(ctx context.Context, app *application.Application, primaryClient *identity.Client) {
	log := logr.FromContextOrDiscard(ctx)

	// If the identity client has graceful rotation disabled, the old secret
	// was replaced immediately — no expiry timestamps will ever arrive.
	// Complete the rotation without waiting.
	if primaryClient != nil && !primaryClient.SupportsSecretRotation() {
		log.Info("Secret rotation: graceful rotation disabled on identity client, completing immediately",
			"application", app.Name,
		)
		app.Status.RotatedExpiresAt = nil
	} else if app.Status.RotatedExpiresAt == nil {
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
	identityProvider, err := zone.Spec.GetIdentityProvider()
	if err != nil {
		log.Error(err, "Cannot schedule secret-rotation notification")
		return
	}
	rotCfg := identityProvider.SecretRotation
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

	// Resolve realm name from zone status (decoupled from environment name)
	if zone.Status.IdentityRealm == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %s has no IdentityRealm status set", zone.Name)
	}
	realmName := zone.Status.IdentityRealm.Name
	namespace := zone.Status.Namespace

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
		owner.Status.RotatedExpiresAt = idpClient.Status.RotatedSecretExpiresAt

		if owner.Spec.RotatedSecret != "" {
			// this value is set in the application-webhook if graceful-secret-rotation is used
			owner.Status.RotatedClientSecret = owner.Spec.RotatedSecret
		} else {
			// only as fallback, this field is only set in identity when no secret-manager is configured.
			owner.Status.RotatedClientSecret = idpClient.Status.RotatedClientSecret
		}
	}

	return idpClient, nil
}

func CreateGatewayConsumers(ctx context.Context, zone *admin.Zone, owner *application.Application, gateways []*admin.GatewayConfig, opts ...CreateOption) error {
	options := &CreateOptions{}
	for _, opt := range opts {
		opt(options)
	}

	c := client.ClientFromContextOrDie(ctx)
	clientId := MakeClientName(owner)

	for _, gatewayConfig := range gateways {
		gatewayStatus, err := zone.Status.GetGateway(gatewayConfig.Name)
		if err != nil {
			return ctrlerrors.BlockedErrorf("zone %q does not contain status for gateway %q", zone.Name, gatewayConfig.Name)
		}
		if gatewayStatus.Gateway == nil {
			return ctrlerrors.BlockedErrorf("zone %q gateway %q has no Gateway reference", zone.Name, gatewayStatus.Name)
		}

		resourceName := clientId + "--" + zone.Name + "--" + gatewayStatus.Name
		if len(resourceName) > labelutil.MaxNameLength {
			return ctrlerrors.BlockedErrorf("Gateway Consumer resource name %q exceeds the Kubernetes maximum of %d characters", resourceName, labelutil.MaxNameLength)
		}
		gatewayRef := *gatewayStatus.Gateway
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
				config.BuildLabelKey("gateway"):     gatewayRef.Name,
				config.BuildLabelKey("zone"):        zone.Name,
			}
			if options.Failover {
				consumer.Labels[config.BuildLabelKey("failover")] = "true"
			}

			if refErr := ctrl.SetControllerReference(owner, consumer, c.Scheme()); refErr != nil {
				return errors.Wrapf(refErr, "failed to set controller reference for gateway consumer %s", resourceName)
			}
			consumer.Spec = gateway.ConsumerSpec{
				Gateway: gatewayRef,
				Name:    clientId,
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

		if _, err := c.CreateOrUpdate(ctx, consumer, mutator); err != nil {
			return errors.Wrapf(err, "failed to create or update Gateway Consumer %s", resourceName)
		}

		owner.Status.Consumers = append(owner.Status.Consumers, *types.ObjectRefFromObject(consumer))
	}
	slices.SortFunc(owner.Status.Consumers, func(a, b types.ObjectRef) int {
		if namespaceOrder := strings.Compare(a.Namespace, b.Namespace); namespaceOrder != 0 {
			return namespaceOrder
		}
		return strings.Compare(a.Name, b.Name)
	})

	return nil
}
