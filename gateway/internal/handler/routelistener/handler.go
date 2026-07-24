// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package routelistener

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/handler/route"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var _ handler.Handler[*v1.RouteListener] = &RouteListenerHandler{}

type RouteListenerHandler struct{}

func (h *RouteListenerHandler) CreateOrUpdate(ctx context.Context, routeListener *v1.RouteListener) error {
	ready, _, err := route.GetRouteByRef(ctx, routeListener.Spec.Route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			routeListener.SetCondition(condition.NewBlockedCondition("Route not found"))
			routeListener.SetCondition(condition.NewNotReadyCondition("RouteNotFound", "Route not found"))
			return nil
		}
		return errors.Wrap(err, "failed to get route by ref")
	}
	if !ready {
		routeListener.SetCondition(condition.NewBlockedCondition("Route not ready"))
		routeListener.SetCondition(condition.NewNotReadyCondition("RouteNotReady", "Route is not ready"))
		return nil
	}

	routeListener.SetCondition(condition.NewDoneProcessingCondition("RouteListener is ready"))
	routeListener.SetCondition(condition.NewReadyCondition("RouteListenerReady", "RouteListener is ready"))
	return nil
}

func (h *RouteListenerHandler) Delete(ctx context.Context, routeListener *v1.RouteListener) error {
	log := log.FromContext(ctx)
	log.Info("Handling deletion of RouteListener resource", "routeListener", routeListener.Name)

	return nil
}
