// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rover

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover/internal/handler/rover/api"
	"github.com/telekom/controlplane/rover/internal/handler/rover/application"
	"github.com/telekom/controlplane/rover/internal/handler/rover/event"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

var _ handler.Handler[*roverv1.Rover] = (*RoverHandler)(nil)

type RoverHandler struct{}

func (h *RoverHandler) CreateOrUpdate(ctx context.Context, roverObj *roverv1.Rover) error {
	c := client.ClientFromContextOrDie(ctx)
	log := logr.FromContextOrDiscard(ctx)
	c.AddKnownTypeToState(&apiapi.ApiExposure{})
	c.AddKnownTypeToState(&apiapi.ApiSubscription{})
	if config.FeaturePubSub.IsEnabled() {
		c.AddKnownTypeToState(&eventv1.EventExposure{})
		c.AddKnownTypeToState(&eventv1.EventSubscription{})
	}

	// Create Application from Rover
	err := application.HandleApplication(ctx, c, roverObj)
	if err != nil {
		return errors.Wrap(err, "failed to handle application")
	}

	// Handle exposures
	roverObj.Status.ApiExposures = make([]types.ObjectRef, 0, len(roverObj.Spec.Exposures))
	roverObj.Status.EventExposures = make([]types.ObjectRef, 0, len(roverObj.Spec.Exposures))
	seenDescriminators := make(map[string]struct{})

	for _, exp := range roverObj.Spec.Exposures {
		switch exp.Type() {
		case roverv1.TypeApi:
			if _, exists := seenDescriminators[exp.Api.BasePath]; exists {
				return ctrlerrors.BlockedErrorf("duplicate API base path in exposures: %s", exp.Api.BasePath)
			}
			seenDescriminators[exp.Api.BasePath] = struct{}{}
			err := api.HandleExposure(ctx, c, roverObj, exp.Api)
			if err != nil {
				return errors.Wrap(err, "failed to handle exposure")
			}

		case roverv1.TypeEvent:
			if _, exists := seenDescriminators[exp.Event.EventType]; exists {
				return ctrlerrors.BlockedErrorf("duplicate event type in exposures: %s", exp.Event.EventType)
			}
			seenDescriminators[exp.Event.EventType] = struct{}{}
			if !config.FeaturePubSub.IsEnabled() {
				log.Info("event exposure skipped, feature has not been enabled")
				continue
			}
			err := event.HandleExposure(ctx, c, roverObj, exp.Event)
			if err != nil {
				return errors.Wrap(err, "failed to handle event exposure")
			}

		default:
			return errors.New("unknown exposure type: " + exp.Type().String())
		}
	}

	// Handle subscriptions
	roverObj.Status.ApiSubscriptions = make([]types.ObjectRef, 0, len(roverObj.Spec.Subscriptions))
	roverObj.Status.EventSubscriptions = make([]types.ObjectRef, 0, len(roverObj.Spec.Subscriptions))
	for _, sub := range roverObj.Spec.Subscriptions {
		switch sub.Type() {
		case roverv1.TypeApi:
			err := api.HandleSubscription(ctx, c, roverObj, sub.Api)
			if err != nil {
				return errors.Wrap(err, "failed to handle subscription")
			}

		case roverv1.TypeEvent:
			if !config.FeaturePubSub.IsEnabled() {
				log.Info("event subscription skipped, feature has not been enabled")
				continue
			}
			err := event.HandleSubscription(ctx, c, roverObj, sub.Event)
			if err != nil {
				return errors.Wrap(err, "failed to handle event subscription")
			}

		default:
			return errors.New("unknown subscription type: " + sub.Type().String())
		}
	}

	// Cleanup all objects owned by Rover
	if _, err = c.CleanupAll(ctx, client.OwnedBy(roverObj)); err != nil {
		return errors.Wrap(err, "failed to cleanup all")
	}

	roverObj.SetCondition(
		condition.NewDoneProcessingCondition("Provisioned all sub-resources"))

	if c.AllReady() {
		roverObj.SetCondition(condition.NewReadyCondition("ProvisioningDone", "All sub-resources are up to date"))

	} else {
		roverObj.SetCondition(
			condition.NewNotReadyCondition("SubResourceNotReady", "At least one sub-resource is being processed"))
	}
	return nil
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
			rover.SetCondition(condition.NewNotReadyCondition("DeletionFailed", "Failed to delete application from secret manager"))
			return errors.Wrap(err, "failed to delete application from secret manager")
		}
	}

	return nil
}
