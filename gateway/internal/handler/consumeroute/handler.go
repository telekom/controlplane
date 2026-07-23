// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ConsumeRoute

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	routehandler "github.com/telekom/controlplane/gateway/internal/handler/route"
)

var _ handler.Handler[*v1.ConsumeRoute] = &ConsumeRouteHandler{}

type ConsumeRouteHandler struct{}

func (h *ConsumeRouteHandler) CreateOrUpdate(ctx context.Context, consumeRoute *v1.ConsumeRoute) error {
	ready, referencedRoute, err := routehandler.GetRouteByRef(ctx, consumeRoute.Spec.Route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			consumeRoute.SetCondition(condition.NewBlockedCondition("Route not found"))
			consumeRoute.SetCondition(condition.NewNotReadyCondition("RouteNotFound", "Route not found"))
			return nil
		}
		return errors.Wrap(err, "failed to get route by ref")
	}
	if !ready {
		consumeRoute.SetCondition(condition.NewBlockedCondition("Route not ready"))
		consumeRoute.SetCondition(condition.NewNotReadyCondition("RouteNotReady", "Route is not ready"))
		return nil
	}

	if slices.Contains(referencedRoute.Status.Consumers, consumeRoute.Spec.ConsumerName) {
		consumeRoute.SetCondition(condition.NewDoneProcessingCondition("ConsumeRoute is ready"))
		consumeRoute.SetCondition(condition.NewReadyCondition("ConsumeRouteReady", "ConsumeRoute is ready"))
		return nil
	}
	consumeRoute.SetCondition(condition.NewProcessingCondition("ConsumeRouteProcessing", "Waiting for Route to be processed"))
	consumeRoute.SetCondition(condition.NewNotReadyCondition("ConsumeRouteProcessing", "Waiting for Route to be processed"))

	return nil
}

func (h *ConsumeRouteHandler) Delete(ctx context.Context, consumeRoute *v1.ConsumeRoute) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion of ConsumeRoute resource")

	return nil
}
