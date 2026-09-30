// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ConsumeRoute

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/handler/route"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const basicWithScopesPolicyMessage = "Consumer username/password with scopes requires an external IDP grant type \"password\""

var _ handler.Handler[*v1.ConsumeRoute] = &ConsumeRouteHandler{}

type ConsumeRouteHandler struct{}

func (h *ConsumeRouteHandler) CreateOrUpdate(ctx context.Context, consumeRoute *v1.ConsumeRoute) error {
	ready, route, err := route.GetRouteByRef(ctx, consumeRoute.Spec.Route)
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
	if violatesBasicWithScopesPolicy(consumeRoute, route) {
		consumeRoute.SetCondition(condition.NewBlockedCondition(basicWithScopesPolicyMessage))
		consumeRoute.SetCondition(condition.NewNotReadyCondition(condition.ReasonValidationFailed, basicWithScopesPolicyMessage))
		return ctrlerrors.BlockedErrorf("%s", basicWithScopesPolicyMessage)
	}

	if slices.Contains(route.Status.Consumers, consumeRoute.Spec.ConsumerName) {
		consumeRoute.SetCondition(condition.NewDoneProcessingCondition("ConsumeRoute is ready"))
		consumeRoute.SetCondition(condition.NewReadyCondition("ConsumeRouteReady", "ConsumeRoute is ready"))
		return nil
	}
	consumeRoute.SetCondition(condition.NewProcessingCondition("ConsumeRouteProcessing", "Waiting for Route to be processed"))
	consumeRoute.SetCondition(condition.NewNotReadyCondition("ConsumeRouteProcessing", "Waiting for Route to be processed"))

	return nil
}

func violatesBasicWithScopesPolicy(consumeRoute *v1.ConsumeRoute, route *v1.Route) bool {
	if !route.IsPrimary() && !route.IsFailoverSecondary() {
		return false
	}
	if !consumeRoute.HasM2MBasic() || len(consumeRoute.Spec.Security.M2M.Scopes) == 0 {
		return false
	}

	security := route.Spec.Security
	if route.IsFailoverSecondary() && route.Spec.Traffic.Failover != nil {
		security = route.Spec.Traffic.Failover.Security
	}
	return !security.HasM2MExternalIDP() || security.M2M.ExternalIDP.GrantType != v1.GrantTypePassword
}

func (h *ConsumeRouteHandler) Delete(ctx context.Context, consumeRoute *v1.ConsumeRoute) error {
	log := log.FromContext(ctx)
	log.Info("Handing deletion of ConsumeRoute resource", "consumeRoute", consumeRoute)

	return nil
}
