// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rover

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/rover/ai"
	"github.com/telekom/controlplane/rover/internal/handler/rover/api"
	"github.com/telekom/controlplane/rover/internal/handler/rover/application"
	"github.com/telekom/controlplane/rover/internal/handler/rover/event"
	"github.com/telekom/controlplane/rover/internal/handler/rover/permission"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

var _ handler.Handler[*roverv1.Rover] = (*RoverHandler)(nil)

type RoverHandler struct{}

func (h *RoverHandler) CreateOrUpdate(ctx context.Context, roverObj *roverv1.Rover) error {
	c := client.ClientFromContextOrDie(ctx)
	logger := logr.FromContextOrDiscard(ctx)
	addKnownTypes(c)

	// Create Application from Rover
	if err := application.HandleApplication(ctx, c, roverObj); err != nil {
		return errors.Wrap(err, "failed to handle application")
	}

	if err := h.handleExposures(ctx, c, roverObj, logger); err != nil {
		return err
	}

	if err := h.handleSubscriptions(ctx, c, roverObj, logger); err != nil {
		return err
	}

	if err := h.handlePermissions(ctx, c, roverObj, logger); err != nil {
		return err
	}

	// Cleanup all objects owned by Rover
	if _, err := c.CleanupAll(ctx, client.OwnedBy(roverObj)); err != nil {
		return errors.Wrap(err, "failed to cleanup all")
	}

	setRoverConditions(c, roverObj)
	return nil
}

func addKnownTypes(c client.JanitorClient) {
	c.AddKnownTypeToState(&apiapi.ApiExposure{})
	c.AddKnownTypeToState(&apiapi.ApiSubscription{})
	if config.FeaturePubSub.IsEnabled() {
		c.AddKnownTypeToState(&eventv1.EventExposure{})
		c.AddKnownTypeToState(&eventv1.EventSubscription{})
	}
	if config.FeaturePermission.IsEnabled() {
		c.AddKnownTypeToState(&permissionv1.PermissionSet{})
	}
	if config.FeatureAiGateway.IsEnabled() {
		c.AddKnownTypeToState(&agenticv1.AgenticExposure{})
		c.AddKnownTypeToState(&agenticv1.AgenticSubscription{})
	}
}

func (h *RoverHandler) handleExposures(ctx context.Context, c client.JanitorClient, roverObj *roverv1.Rover, logger logr.Logger) error {
	roverObj.Status.ApiExposures = make([]types.ObjectRef, 0, len(roverObj.Spec.Exposures))
	roverObj.Status.EventExposures = make([]types.ObjectRef, 0, len(roverObj.Spec.Exposures))
	roverObj.Status.AgenticExposures = make([]types.ObjectRef, 0, len(roverObj.Spec.Exposures))

	seenDiscriminators := make(map[string]struct{})
	for _, exp := range roverObj.Spec.Exposures {
		if err := h.handleExposure(ctx, c, roverObj, exp, seenDiscriminators, logger); err != nil {
			return err
		}
	}

	return nil
}

func (h *RoverHandler) handleExposure(ctx context.Context, c client.JanitorClient, roverObj *roverv1.Rover, exp roverv1.Exposure, seenDiscriminators map[string]struct{}, logger logr.Logger) error {
	switch exp.Type() {
	case roverv1.TypeApi:
		if err := recordUniqueDiscriminator(seenDiscriminators, exp.Api.BasePath, "duplicate API base path in exposures: %s"); err != nil {
			return err
		}
		if err := api.HandleExposure(ctx, c, roverObj, exp.Api); err != nil {
			return errors.Wrap(err, "failed to handle exposure")
		}
	case roverv1.TypeEvent:
		if err := recordUniqueDiscriminator(seenDiscriminators, exp.Event.EventType, "duplicate event type in exposures: %s"); err != nil {
			return err
		}
		if !config.FeaturePubSub.IsEnabled() {
			logger.Info("event exposure skipped, feature has not been enabled")
			return nil
		}
		if err := event.HandleExposure(ctx, c, roverObj, exp.Event); err != nil {
			return errors.Wrap(err, "failed to handle event exposure")
		}
	case roverv1.TypeAgentic:
		if err := recordUniqueDiscriminator(seenDiscriminators, exp.Agentic.BasePath, "duplicate AI base path in exposures: %s"); err != nil {
			return err
		}
		if !config.FeatureAiGateway.IsEnabled() {
			logger.Info("AI exposure skipped, feature has not been enabled")
			return nil
		}
		if err := ai.HandleExposure(ctx, c, roverObj, exp.Agentic); err != nil {
			return errors.Wrap(err, "failed to handle AI exposure")
		}
	default:
		return errors.New("unknown exposure type: " + exp.Type().String())
	}

	return nil
}

func (h *RoverHandler) handleSubscriptions(ctx context.Context, c client.JanitorClient, roverObj *roverv1.Rover, logger logr.Logger) error {
	roverObj.Status.ApiSubscriptions = make([]types.ObjectRef, 0, len(roverObj.Spec.Subscriptions))
	roverObj.Status.EventSubscriptions = make([]types.ObjectRef, 0, len(roverObj.Spec.Subscriptions))
	roverObj.Status.AgenticSubscriptions = make([]types.ObjectRef, 0, len(roverObj.Spec.Subscriptions))

	for _, sub := range roverObj.Spec.Subscriptions {
		if err := h.handleSubscription(ctx, c, roverObj, sub, logger); err != nil {
			return err
		}
	}

	return nil
}

func (h *RoverHandler) handleSubscription(ctx context.Context, c client.JanitorClient, roverObj *roverv1.Rover, sub roverv1.Subscription, logger logr.Logger) error {
	switch sub.Type() {
	case roverv1.TypeApi:
		if err := api.HandleSubscription(ctx, c, roverObj, sub.Api); err != nil {
			return errors.Wrap(err, "failed to handle subscription")
		}
	case roverv1.TypeEvent:
		if !config.FeaturePubSub.IsEnabled() {
			logger.Info("event subscription skipped, feature has not been enabled")
			return nil
		}
		if err := event.HandleSubscription(ctx, c, roverObj, sub.Event); err != nil {
			return errors.Wrap(err, "failed to handle event subscription")
		}
	case roverv1.TypeAgentic:
		if !config.FeatureAiGateway.IsEnabled() {
			logger.Info("AI subscription skipped, feature has not been enabled")
			return nil
		}
		if err := ai.HandleSubscription(ctx, c, roverObj, sub.Agentic); err != nil {
			return errors.Wrap(err, "failed to handle AI subscription")
		}
	default:
		return errors.New("unknown subscription type: " + sub.Type().String())
	}

	return nil
}

func (h *RoverHandler) handlePermissions(ctx context.Context, c client.JanitorClient, roverObj *roverv1.Rover, logger logr.Logger) error {
	roverObj.Status.PermissionSets = make([]types.ObjectRef, 0)
	if len(roverObj.Spec.Permissions) == 0 {
		return nil
	}
	if !config.FeaturePermission.IsEnabled() {
		logger.Info("permission handling skipped, feature has not been enabled")
		return nil
	}

	if err := permission.HandlePermission(ctx, c, roverObj); err != nil {
		return errors.Wrap(err, "failed to handle permission")
	}

	return nil
}

func recordUniqueDiscriminator(seen map[string]struct{}, discriminator, message string) error {
	if _, exists := seen[discriminator]; exists {
		return ctrlerrors.BlockedErrorf(message, discriminator)
	}
	seen[discriminator] = struct{}{}

	return nil
}

func setRoverConditions(c client.JanitorClient, roverObj *roverv1.Rover) {
	roverObj.SetCondition(condition.NewDoneProcessingCondition("Provisioned all sub-resources"))

	if c.AllReady() {
		roverObj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "All sub-resources are up to date"))
		return
	}

	roverObj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, "At least one sub-resource is being processed"))
}

func (h *RoverHandler) Delete(ctx context.Context, rover *roverv1.Rover) error {
	if config.FeatureSecretManager.IsEnabled() {
		envId := contextutil.EnvFromContextOrDie(ctx)
		parts := strings.SplitN(rover.GetNamespace(), "--", 2)
		teamId := parts[1]
		appId := rover.GetName()
		err := secretsapi.API().DeleteApplication(ctx, envId, teamId, appId)
		if err != nil {
			// If this fails, we have an internal problem
			rover.SetCondition(condition.NewNotReadyCondition(condition.ReasonError, "Failed to delete application from secret manager"))
			return errors.Wrap(err, "failed to delete application from secret manager")
		}
	}

	return nil
}
