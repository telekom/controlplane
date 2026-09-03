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
	ready, resolvedRoute, err := route.GetRouteByRef(ctx, routeListener.Spec.Route)
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

	// Reject pass-through and failover routes: pass-through skips
	// authentication (the route handler excludes RouteListeners inside
	// `if !route.Spec.PassThrough`), and failover overwrites /listener
	// upstream to /proxy (priority 109 > 103).
	if resolvedRoute.Spec.PassThrough {
		routeListener.SetCondition(condition.NewBlockedCondition("Route is pass-through — listener capture is not supported"))
		routeListener.SetCondition(condition.NewNotReadyCondition("RouteUnsupported", "Route is pass-through — listener capture is not supported"))
		return nil
	}
	if resolvedRoute.Spec.Traffic.Failover != nil {
		routeListener.SetCondition(condition.NewBlockedCondition("Route has failover — listener capture is not supported"))
		routeListener.SetCondition(condition.NewNotReadyCondition("RouteUnsupported", "Route has failover — listener capture is not supported"))
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
