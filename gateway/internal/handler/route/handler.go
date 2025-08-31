// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"

	"github.com/pkg/errors"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/internal/handler/realm"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*gatewayv1.Route] = &RouteHandler{}

type RouteHandler struct{}

func (h *RouteHandler) CreateOrUpdate(ctx context.Context, route *gatewayv1.Route) error {
	log := log.FromContext(ctx)
	kubeClient := cc.ClientFromContextOrDie(ctx)
	builder, err := NewFeatureBuilder(ctx, route)
	if err != nil {
		return errors.Wrap(err, "failed to create feature builder")
	}

	routeConsumers := &gatewayv1.ConsumeRouteList{}
	if !route.Spec.PassThrough {
		listOpts := []client.ListOption{}

		// If this is a proxy-route, we only need the consumers which are directly associated
		// with this route as we just need to add them to the ACL plugin.
		if route.IsProxy() && !route.IsFailoverSecondary() {
			log.Info("Route is a proxy route, only looking for direct consumers")
			listOpts = append(listOpts, client.MatchingFields{
				// This index field is defined in internal/controller/index.go
				"spec.route": types.ObjectRefFromObject(route).String(),
			})
		} else {
			// We need to get all Consumers that want to consume this Route
			listOpts = append(listOpts,
				client.MatchingFields{
					// This index field is defined in internal/controller/index.go
					"spec.route.name": route.Name,
				})

			log.Info("Route is not a proxy route, looking for all consumers")
		}
		// If this is not a proxy-route, we need all consumers as we need to add their security-config
		// to the JumperConfig

		err = kubeClient.List(ctx, routeConsumers, listOpts...)
		if err != nil {
			return errors.Wrap(err, "failed to list route consumers")
		}

		for _, consumer := range routeConsumers.Items {
			if controller.IsBeingDeleted(&consumer) {
				log.V(1).Info("Skipping consumer that is being deleted", "consumer", consumer.Name)
				continue
			}
			builder.AddAllowedConsumers(&consumer)
		}
		log.Info("Found consumers", "count", len(builder.GetAllowedConsumers()), "sum", len(routeConsumers.Items))

	}

	if err := builder.Build(ctx); err != nil {
		return errors.Wrap(err, "failed to build route")
	}

	// Reset the consumers list to only contain the current consumer names
	route.Status.Consumers = []string{}
	for _, consumer := range builder.GetAllowedConsumers() {
		// We needed all consumers for real-routes to construct the JumperConfig,
		// but we only want to add the consumers that are actually consuming this route
		// to the route status.
		if consumer.Spec.Route.Equals(route) {
			route.Status.Consumers = append(route.Status.Consumers, consumer.Spec.ConsumerName)
		}
	}

	route.SetCondition(condition.NewReadyCondition("RouteProcessed", "Route processed successfully"))
	route.SetCondition(condition.NewDoneProcessingCondition("Route processed successfully"))

	return nil
}

func (h *RouteHandler) Delete(ctx context.Context, route *gatewayv1.Route) error {

	_, realm, err := realm.GetRealmByRef(ctx, route.Spec.Realm)
	if err != nil {
		return err
	}

	_, gateway, err := gateway.GetGatewayByRef(ctx, *realm.Spec.Gateway, true)
	if err != nil {
		return err
	}

	kc, err := kongutil.GetClientFor(gateway)
	if err != nil {
		return errors.Wrap(err, "failed to get kong client")
	}

	err = kc.DeleteRoute(ctx, route)
	if err != nil {
		return errors.Wrap(err, "failed to delete route")
	}

	return nil
}

func NewFeatureBuilder(ctx context.Context, route *gatewayv1.Route) (features.FeaturesBuilder, error) {
	ready, realm, err := realm.GetRealmByRef(ctx, route.Spec.Realm)
	if err != nil {
		return nil, err
	}
	if !ready {
		route.SetCondition(condition.NewBlockedCondition("Realm is not ready"))
		route.SetCondition(condition.NewNotReadyCondition("RealmNotReady", "Realm is not ready"))
		return nil, nil
	}

	ready, gateway, err := gateway.GetGatewayByRef(ctx, *realm.Spec.Gateway, true)
	if err != nil {
		return nil, err
	}
	if !ready {
		route.SetCondition(condition.NewBlockedCondition("Gateway is not ready"))
		route.SetCondition(condition.NewNotReadyCondition("GatewayNotReady", "Gateway is not ready"))
		return nil, nil
	}

	kc, err := kongutil.GetClientFor(gateway)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kong client")
	}

	builder := features.NewFeatureBuilder(kc, route, nil, realm, gateway)
	builder.EnableFeature(feature.InstanceAccessControlFeature)
	builder.EnableFeature(feature.InstancePassThroughFeature)
	builder.EnableFeature(feature.InstanceLastMileSecurityFeature)
	builder.EnableFeature(feature.InstanceCustomScopesFeature)
	builder.EnableFeature(feature.InstanceLoadBalancingFeature)
	builder.EnableFeature(feature.InstanceExternalIDPFeature)
	builder.EnableFeature(feature.InstanceRateLimitFeature)
	builder.EnableFeature(feature.InstanceFailoverFeature)
	builder.EnableFeature(feature.InstanceHeaderTransformationFeature)
	builder.EnableFeature(feature.InstanceBasicAuthFeature)
	builder.EnableFeature(feature.InstanceCircuitBreakerFeature)

	return builder, nil
}
