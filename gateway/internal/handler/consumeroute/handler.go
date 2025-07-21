// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ConsumeRoute

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/internal/handler/realm"
	"github.com/telekom/controlplane/gateway/internal/handler/route"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

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
		return err
	}
	if !ready {
		consumeRoute.SetCondition(condition.NewBlockedCondition("Route not ready"))
		consumeRoute.SetCondition(condition.NewNotReadyCondition("RouteNotReady", "Route is not ready"))
		return nil
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

func (h *ConsumeRouteHandler) Delete(ctx context.Context, consumeRoute *v1.ConsumeRoute) error {
	log := log.FromContext(ctx)
	log.Info("Handing deletion of ConsumeRoute resource", "consumeRoute", consumeRoute)

	ready, route, err := route.GetRouteByRef(ctx, consumeRoute.Spec.Route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if !ready {
		log.Info("Route is not ready")
		return nil
	}

	_, realm, err := realm.GetRealmByRef(ctx, route.Spec.Realm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	_, gateway, err := gateway.GetGatewayByRef(ctx, *realm.Spec.Gateway, true)
	if err != nil {
		return err
	}

	kc, err := kongutil.GetClientFor(gateway)
	if err != nil {
		return errors.Wrap(err, "failed to get kong client") // internal problem
	}

	aclPlugin := plugin.AclPluginFromRoute(route)
	_, err = kc.LoadPlugin(ctx, aclPlugin, true)
	if err != nil {
		return errors.Wrap(err, "failed to load acl plugin")
	}

	aclPlugin.Config.Allow.Remove(consumeRoute.Spec.ConsumerName)

	_, err = kc.CreateOrReplacePlugin(ctx, aclPlugin)
	if err != nil {
		return errors.Wrap(err, "failed to create or replace acl plugin")
	}

	return nil
}
