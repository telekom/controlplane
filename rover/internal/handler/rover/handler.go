// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rover

import (
	"context"
	"github.com/telekom/controlplane/rover/internal/handler/rover/api"
	"github.com/telekom/controlplane/rover/internal/handler/rover/application"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.Rover] = (*RoverHandler)(nil)

type RoverHandler struct{}

func (h *RoverHandler) CreateOrUpdate(ctx context.Context, roverObj *roverv1.Rover) error {

	log := log.FromContext(ctx)
	log.Info("RoverHandler CreateOrUpdate", "rover", roverObj)

	roverObj.Status = roverv1.RoverStatus{}

	c := client.ClientFromContextOrDie(ctx)
	c.AddKnownTypeToState(&apiapi.ApiExposure{})
	c.AddKnownTypeToState(&apiapi.ApiSubscription{})

	// Create Application from Rover
	err := application.HandleApplication(ctx, c, roverObj)
	if err != nil {
		return errors.Wrap(err, "failed to handle application")
	}

	// Handle exposures
	for _, exp := range roverObj.Spec.Exposures {
		switch exp.Type() {
		case roverv1.TypeApi:
			err := api.HandleExposure(ctx, c, roverObj, exp.Api)
			if err != nil {
				return errors.Wrap(err, "failed to handle exposure")
			}

		case roverv1.TypeEvent:
			return errors.New("event exposure not implemented")

		default:
			return errors.New("unknown exposure type: " + exp.Type().String())
		}
	}

	// Handle subscriptions
	for _, sub := range roverObj.Spec.Subscriptions {
		switch sub.Type() {
		case roverv1.TypeApi:
			err := api.HandleSubscription(ctx, c, roverObj, sub.Api)
			if err != nil {
				return errors.Wrap(err, "failed to handle subscription")
			}

		case roverv1.TypeEvent:
			return errors.New("event subscription not implemented")

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
			condition.NewNotReadyCondition("SubResourceNotReady", "Atleast one sub-resource is being processed"))
	}
	return nil
}

func (h *RoverHandler) Delete(ctx context.Context, rover *roverv1.Rover) error {
	return nil
}
