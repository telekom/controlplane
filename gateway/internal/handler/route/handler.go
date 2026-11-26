// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/changeset"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	gatewayhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*gatewayv1.Route] = &RouteHandler{}

type RouteHandler struct {
	// KongClients provides leased Kong clients for Gateways.
	KongClients *kongutil.ClientFactory
}

func (h *RouteHandler) CreateOrUpdate(ctx context.Context, route *gatewayv1.Route) error {
	// The Kong changes of the Route are planned, decided on as one bundle and only then written.
	if err := changeset.Run(ctx, kongutil.Gate(), route, "Route", h.reconcileKong); err != nil {
		return err
	}

	route.SetCondition(condition.NewReadyCondition("RouteProcessed", "Route processed successfully"))
	route.SetCondition(condition.NewDoneProcessingCondition("Route processed successfully"))

	return nil
}

// reconcileKong builds the Kong entities of route and records its consumers in the status.
// In a ctx with a plan, it records the Kong changes without writing them.
func (h *RouteHandler) reconcileKong(ctx context.Context, route *gatewayv1.Route) error {
	builder, err := h.build(ctx, route)
	if err != nil {
		return err
	}
	ctrllog.FromContext(ctx).V(1).Info("route properties are", "properties", route.Status.Properties)

	// Reset the consumers list to only contain the current consumer names
	route.Status.Consumers = []string{}
	allowedConsumers := builder.GetAllowedConsumers()
	cmpFunc := func(a, b *gatewayv1.ConsumeRoute) int {
		return cmp.Compare(a.Name, b.Name)
	}
	slices.SortStableFunc(allowedConsumers, cmpFunc)

	for _, consumer := range allowedConsumers {
		// We needed all consumers for real-routes to construct the JumperConfig,
		// but we only want to add the consumers that are actually consuming this route
		// to the route status.
		if consumer.Spec.Route.Equals(route) {
			route.Status.Consumers = append(route.Status.Consumers, consumer.Spec.ConsumerName)
		}
	}
	return nil
}

// build builds the Kong entities of route. In a ctx with a plan, it records them without
// writing.
func (h *RouteHandler) build(ctx context.Context, route *gatewayv1.Route) (features.FeaturesBuilder, error) {
	log := ctrllog.FromContext(ctx)
	// The common controller puts a client scoped to the Route's environment into ctx.
	kubeClient := cc.ClientFromContextOrDie(ctx)
	builder, release, err := h.newFeatureBuilder(ctx, route)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create feature builder")
	}
	defer release()

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
			return nil, errors.Wrap(err, "failed to list route consumers")
		}

		for i := range routeConsumers.Items {
			consumer := &routeConsumers.Items[i]
			if controller.IsBeingDeleted(consumer) {
				log.V(1).Info("Skipping consumer that is being deleted", "consumer", consumer.Name)
				continue
			}
			builder.AddAllowedConsumers(consumer)
		}
		log.Info("Found consumers", "count", len(builder.GetAllowedConsumers()), "sum", len(routeConsumers.Items))

		// List RouteListeners for this Route.
		// Skip RouteListeners on routes with failover config — the failover
		// feature (priority 109) would overwrite the /listener upstream
		// (priority 103) to /proxy, silently breaking capture.
		if route.Spec.Traffic.Failover == nil {
			routeListeners := &gatewayv1.RouteListenerList{}
			if err := kubeClient.List(ctx, routeListeners, client.MatchingFields{
				"spec.route": types.ObjectRefFromObject(route).String(),
			}); err != nil {
				return nil, errors.Wrap(err, "failed to list route listeners")
			}
			slices.SortFunc(routeListeners.Items, func(a, b gatewayv1.RouteListener) int {
				return cmp.Compare(a.Name, b.Name)
			})
			for i := range routeListeners.Items {
				if controller.IsBeingDeleted(&routeListeners.Items[i]) {
					continue
				}
				builder.AddRouteListeners(&routeListeners.Items[i])
			}
		}
	}

	if err := builder.Build(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to build route")
	}
	return builder, nil
}

func (h *RouteHandler) Delete(ctx context.Context, route *gatewayv1.Route) error {
	ready, gateway, err := gatewayhandler.GetGatewayByRef(ctx, route.Spec.GatewayRef, true)
	if err != nil {
		return err
	}
	if !ready {
		return ctrlerrors.BlockedErrorf("gateway %q is not ready", route.Spec.GatewayRef.Name)
	}
	// Without a prefix, the Kong client would look the entities up by metadata.name only and
	// leave the prefixed ones behind, so a Route moved to an unprefixed Gateway keeps its prefix.
	prefix := gateway.Spec.NamePrefix
	if prefix == "" {
		prefix = features.RecordedNamePrefix(route)
	}
	route.SetNamePrefix(prefix)

	kc, release, err := kongutil.GetClientFor(ctx, h.KongClients, gateway)
	if err != nil {
		return errors.Wrap(err, "failed to get kong client")
	}
	defer release()

	return changeset.Run(ctx, kongutil.Gate(), route, "Route", func(ctx context.Context, target *gatewayv1.Route) error {
		if err := kc.DeleteRoute(ctx, target); err != nil {
			return errors.Wrap(err, "failed to delete route")
		}
		return nil
	})
}

// newFeatureBuilder returns a builder using a leased Kong client and the
// function releasing the lease.
func (h *RouteHandler) newFeatureBuilder(ctx context.Context, route *gatewayv1.Route) (features.FeaturesBuilder, func(), error) {
	ready, gateway, err := gatewayhandler.GetGatewayByRef(ctx, route.Spec.GatewayRef, true)
	if err != nil {
		return nil, nil, err
	}
	if !ready {
		return nil, nil, ctrlerrors.BlockedErrorf("gateway %q is not ready", route.Spec.GatewayRef.Name)
	}
	// The Kong names of the Route use the prefix of its Gateway. A removed prefix is not
	// followed: the unprefixed lookup ignores the stored IDs and would create a second set of
	// Kong entities beside the prefixed ones.
	if recorded := features.RecordedNamePrefix(route); recorded != "" && gateway.Spec.NamePrefix == "" {
		return nil, nil, ctrlerrors.BlockedErrorf("route was built with name prefix %q, but gateway %q has none; moving a route to a gateway without name prefix is not supported",
			recorded, gateway.Name)
	}
	route.SetNamePrefix(gateway.Spec.NamePrefix)

	kc, release, err := kongutil.GetClientFor(ctx, h.KongClients, gateway)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get kong client")
	}

	builder := features.NewFeatureBuilder(kc, route, nil, gateway)
	builder.EnableFeature(feature.InstanceAccessControlFeature)
	builder.EnableFeature(feature.InstancePassThroughFeature)
	builder.EnableFeature(feature.InstanceLastMileSecurityFeature)
	builder.EnableFeature(feature.InstanceCustomScopesFeature)
	builder.EnableFeature(feature.InstanceClaimsFeature)
	builder.EnableFeature(feature.InstanceLoadBalancingFeature)
	builder.EnableFeature(feature.InstanceExternalIDPFeature)
	builder.EnableFeature(feature.InstanceRateLimitFeature)
	builder.EnableFeature(feature.InstanceFailoverFeature)
	builder.EnableFeature(feature.InstanceHeaderTransformationFeature)
	builder.EnableFeature(feature.InstanceBasicAuthFeature)
	builder.EnableFeature(feature.InstanceCircuitBreakerFeature)
	builder.EnableFeature(feature.InstanceDynamicUpstreamFeature)
	builder.EnableFeature(feature.InstanceRequestTerminationFeature)
	builder.EnableFeature(feature.InstanceRouteListenerFeature)

	return builder, release, nil
}
