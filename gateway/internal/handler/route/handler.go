// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*gatewayv1.Route] = &RouteHandler{}

type RouteHandler struct{}

func (h *RouteHandler) CreateOrUpdate(ctx context.Context, route *gatewayv1.Route) error {
	logger := log.FromContext(ctx)
	kubeClient := cc.ClientFromContextOrDie(ctx)
	ready, referencedGateway, err := gateway.GetGatewayByRef(ctx, route.Spec.GatewayRef, false)
	if err != nil {
		return errors.Wrap(err, "failed to get gateway")
	}
	if !ready {
		return errors.Wrap(
			ctrlerrors.BlockedErrorf("gateway %q is not ready", route.Spec.GatewayRef.Name),
			"failed to create feature builder",
		)
	}
	if referencedGateway.Spec.Type == gatewayv1.GatewayTypeEnvoy && len(route.Status.Properties) > 0 {
		if !meta.IsStatusConditionTrue(referencedGateway.Status.Conditions, "XDSProgrammed") {
			return ctrlerrors.BlockedErrorf("gateway %q xDS target is not programmed", route.Spec.GatewayRef.Name)
		}
		_, referencedGateway, err = gateway.GetGatewayByRef(ctx, route.Spec.GatewayRef, true)
		if err != nil {
			return errors.Wrap(err, "failed to resolve gateway for Kong cleanup")
		}
		kc, clientErr := kongutil.GetClientFor(referencedGateway)
		if clientErr != nil {
			return errors.Wrap(clientErr, "failed to get Kong client for backend switch")
		}
		if deleteErr := kc.DeleteRoute(ctx, route); deleteErr != nil {
			return errors.Wrap(deleteErr, "failed to clean up Kong route during backend switch")
		}
		route.Status.Properties = map[string]string{}
	}
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
			logger.Info("Route is a proxy route, only looking for direct consumers")
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

			logger.Info("Route is not a proxy route, looking for all consumers")
		}
		// If this is not a proxy-route, we need all consumers as we need to add their security-config
		// to the JumperConfig

		err = kubeClient.List(ctx, routeConsumers, listOpts...)
		if err != nil {
			return errors.Wrap(err, "failed to list route consumers")
		}

		for i := range routeConsumers.Items {
			consumer := &routeConsumers.Items[i]
			if controller.IsBeingDeleted(consumer) {
				logger.V(1).Info("Skipping consumer that is being deleted", "consumer", consumer.Name)
				continue
			}
			builder.AddAllowedConsumers(consumer)
		}
		logger.Info("Found consumers", "count", len(builder.GetAllowedConsumers()), "sum", len(routeConsumers.Items))

	}

	if err := builder.Build(ctx); err != nil {
		return errors.Wrap(err, "failed to build route")
	}
	logger.V(1).Info("route properties are", "properties", route.Status.Properties)

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

	route.SetCondition(condition.NewReadyCondition("RouteProcessed", "Route processed successfully"))
	route.SetCondition(condition.NewDoneProcessingCondition("Route processed successfully"))

	return nil
}

func (h *RouteHandler) Delete(ctx context.Context, route *gatewayv1.Route) error {
	ready, referencedGateway, err := gateway.GetGatewayByRef(ctx, route.Spec.GatewayRef, false)
	if err != nil {
		return err
	}
	if referencedGateway.Spec.Type == gatewayv1.GatewayTypeEnvoy {
		// Gateway reconciliation publishes a complete bundle without this route.
		return nil
	}
	if !ready {
		return ctrlerrors.BlockedErrorf("gateway %q is not ready", route.Spec.GatewayRef.Name)
	}
	_, referencedGateway, err = gateway.GetGatewayByRef(ctx, route.Spec.GatewayRef, true)
	if err != nil {
		return err
	}

	kc, err := kongutil.GetClientFor(referencedGateway)
	if err != nil {
		return errors.Wrap(err, "failed to get kong client")
	}

	err = kc.DeleteRoute(ctx, route)
	if err != nil {
		return errors.Wrap(err, "failed to delete route")
	}

	return nil
}

func NewFeatureBuilder(ctx context.Context, route *gatewayv1.Route) (features.FeatureBuilder, error) {
	ready, referencedGateway, err := gateway.GetGatewayByRef(ctx, route.Spec.GatewayRef, false)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, ctrlerrors.BlockedErrorf("gateway %q is not ready", route.Spec.GatewayRef.Name)
	}

	if referencedGateway.Spec.Type == gatewayv1.GatewayTypeEnvoy {
		builder := envoy.NewFeatureBuilder(nil, route, nil, referencedGateway)
		if len(route.Spec.Backend.Upstreams) == 0 {
			return nil, ctrlerrors.BlockedErrorf("route %q has no upstream", route.Name)
		}
		builder.SetUpstream(route.Spec.Backend.Upstreams[0])
		builder.EnableFeature(envoy.InstanceAccessControlFeature)
		builder.EnableFeature(envoy.InstanceLastMileSecurityFeature)
		return builder, nil
	}
	_, referencedGateway, err = gateway.GetGatewayByRef(ctx, route.Spec.GatewayRef, true)
	if err != nil {
		return nil, err
	}

	kc, err := kongutil.GetClientFor(referencedGateway)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kong client")
	}

	builder := features.NewFeatureBuilder(kc, route, nil, referencedGateway)
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

	return builder, nil
}
