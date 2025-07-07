// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	LabelFailoverTarget = config.BuildLabelKey("failover.target")
)

type CreateRouteOptions struct {
	FailoverUpstreams    []apiapi.Upstream
	FailoverZone         types.ObjectRef
	DeleteFailoverTarget bool
}

type CreateRouteOption func(*CreateRouteOptions)

func WithFailoverUpstreams(failoverUpstreams ...apiapi.Upstream) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.FailoverUpstreams = failoverUpstreams
	}
}

func WithDeleteFailoverTarget() CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.DeleteFailoverTarget = true
	}
}

func WithFailoverZone(failoverZone types.ObjectRef) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.FailoverZone = failoverZone
	}
}

// IsFailoverTarget checks if the route is a failover target.
// This means that this route has the real upstream as a failover target.
func (o *CreateRouteOptions) IsFailoverTarget() bool {
	return len(o.FailoverUpstreams) > 0
}

// HasFailover checks if the route has a failover zone configured.
// This means that this route is used as a proxy to the failover zone.
func (o *CreateRouteOptions) HasFailover() bool {
	return o.FailoverZone.Name != "" && o.FailoverZone.Namespace != ""
}

func MakeRouteName(apiBasePath, realmName string) string {
	routeName := labelutil.NormalizeValue(apiBasePath)
	if realmName != "default" {
		routeName = realmName + "--" + routeName
	}
	return routeName
}

func CreateProxyRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, upstreamZoneRef types.ObjectRef, apiBasePath, realmName string, opts ...CreateRouteOption) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	log := log.FromContext(ctx)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Downstream
	downstreamRealm, downstreamZone, err := GetRealmForZone(ctx, downstreamZoneRef, realmName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get downstream-realm %s", downstreamZoneRef.String())
	}

	// Upstream
	upstreamRealm, upstreamZone, err := GetRealmForZone(ctx, upstreamZoneRef, realmName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get upstream-realm %s", upstreamZoneRef.Name)
	}

	// Creating the Route
	proxyRoute := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeRouteName(apiBasePath, realmName),
			Namespace: downstreamRealm.Namespace,
		},
	}

	mutate := func() error {
		proxyRoute.Labels = map[string]string{
			apiapi.BasePathLabelKey:       labelutil.NormalizeValue(apiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(downstreamZone.GetName()),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(realmName),
			config.BuildLabelKey("type"):  "proxy",
		}

		downstream, err := downstreamRealm.AsDownstream(apiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
		}

		upstream, err := AsUpstreamForProxyRoute(ctx, upstreamRealm, apiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create upstream")
		}

		proxyRoute.Spec = gatewayapi.RouteSpec{
			Realm: *types.ObjectRefFromObject(downstreamRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
		}

		log.Info("Creating proxy route", "route", proxyRoute.Name, "namespace", proxyRoute.Namespace, "failover", options.HasFailover())

		if options.IsFailoverTarget() {
			proxyRoute.Labels[LabelFailoverTarget] = "true"

			failoverUpstreams := make([]gatewayapi.Upstream, 0, len(options.FailoverUpstreams))
			for _, rawUpstream := range options.FailoverUpstreams {
				failoverUpstream, err := AsUpstreamForRealRoute(ctx, rawUpstream.Url)
				if err != nil {
					return errors.Wrapf(err, "failed to create failover upstream %s", rawUpstream.Url)
				}
				failoverUpstreams = append(failoverUpstreams, failoverUpstream)
			}

			proxyRoute.Spec.Traffic = gatewayapi.Traffic{
				Failover: &gatewayapi.Failover{
					TargetZoneName: upstreamZone.Name,
					Upstreams:      failoverUpstreams,
				},
			}
		}

		if options.HasFailover() {
			proxyRoute.Labels[config.BuildLabelKey("failover.zone")] = labelutil.NormalizeValue(options.FailoverZone.Name)
			failoverUpstreamRealm, _, err := GetRealmForZone(ctx, options.FailoverZone, realmName)
			if err != nil {
				return errors.Wrapf(err, "failed to get failover zone %s", options.FailoverZone.String())
			}
			failoverUpstream, err := AsUpstreamForProxyRoute(ctx, failoverUpstreamRealm, apiBasePath)

			proxyRoute.Spec.Traffic = gatewayapi.Traffic{
				Failover: &gatewayapi.Failover{
					TargetZoneName: upstreamZone.Name,
					Upstreams: []gatewayapi.Upstream{
						failoverUpstream,
					},
				},
			}
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, proxyRoute, mutate)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create proxy route")
	}

	return proxyRoute, nil
}

// CleanupProxyRoute deletes the route only if no other subscriptions (size > 1) for this route exist
func CleanupProxyRoute(ctx context.Context, routeRef *types.ObjectRef, opts ...CreateRouteOption) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	if routeRef == nil {
		return nil
	}
	log := log.FromContext(ctx).WithValues("route.name", routeRef.Name, "route.namespace", routeRef.Namespace)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	route := &gatewayapi.Route{}
	err := scopedClient.Get(ctx, routeRef.K8s(), route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to get route")
	}

	if route.GetLabels()[config.BuildLabelKey("type")] == "real" { // DO NOT DELETE REAL ROUTES
		log.V(1).Info("🫷 Not deleting route as it is a real route")
		return nil
	}

	if !options.DeleteFailoverTarget && route.GetLabels()[LabelFailoverTarget] == "true" { // DO NOT DELETE FAILOVER ROUTES
		log.V(1).Info("🫷 Not deleting route as it is a failover target")
		return nil
	}

	basePath := route.GetLabels()[apiapi.BasePathLabelKey]
	zone := route.GetLabels()[config.BuildLabelKey("zone")]

	apiSubscriptions := &apiapi.ApiSubscriptionList{}
	err = scopedClient.List(ctx, apiSubscriptions, client.MatchingLabels{
		apiapi.BasePathLabelKey:      basePath,
		config.BuildLabelKey("zone"): zone,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to list routes matching basePath %s in zone %s", basePath, zone)
	}

	if len(apiSubscriptions.Items) > 1 {
		log.Info("🫷 Not deleting route as more than 1 subscriptions exists")
		return nil
	}

	log.Info("🧹 Deleting route as no more subscriptions exist")

	err = scopedClient.Delete(ctx, route)
	if err != nil {
		return errors.Wrapf(err, "failed to delete route")
	}
	log.Info("✅ Successfully deleted obsolete route")

	return nil
}

func CreateRealRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, upstream, apiBasePath, realmName string) (*gatewayapi.Route, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	// get referenced Zone from exposure
	zone, err := GetZone(ctx, scopedClient, downstreamZoneRef.K8s())
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Unable to get zone %s", downstreamZoneRef.String()))
	}
	downstreamRealmRef := client.ObjectKey{
		Name:      realmName,
		Namespace: zone.Status.Namespace,
	}

	downstreamRealm, err := GetRealm(ctx, downstreamRealmRef)
	if err != nil {
		return nil, err
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeRouteName(apiBasePath, realmName),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			apiapi.BasePathLabelKey:       labelutil.NormalizeValue(apiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(zone.Name),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(downstreamRealm.Name),
			config.BuildLabelKey("type"):  "real",
		}

		downstream, err := downstreamRealm.AsDownstream(apiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
		}

		upstream, err := AsUpstreamForRealRoute(ctx, upstream)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm: *types.ObjectRefFromObject(downstreamRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
			Traffic: gatewayapi.Traffic{},
		}
		return nil
	}

	_, err = scopedClient.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update route: %s in namespace: %s", route.Name, route.Namespace)
	}

	return route, nil
}

func CreateConsumeRoute(ctx context.Context, parent *apiapi.ApiSubscription, downstreamZoneRef types.ObjectRef, routeRef types.ObjectRef, clientId string) (*gatewayapi.ConsumeRoute, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	name := downstreamZoneRef.Name + "--" + parent.GetName()
	routeConsumer := &gatewayapi.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: parent.GetNamespace(),
		},
	}

	mutate := func() error {
		if err := controllerutil.SetControllerReference(parent, routeConsumer, scopedClient.Scheme()); err != nil {
			return errors.Wrapf(err, "failed to set owner-reference on %v", routeConsumer)
		}
		routeConsumer.Labels = parent.GetLabels()

		routeConsumer.Spec = gatewayapi.ConsumeRouteSpec{
			Route:        routeRef,
			ConsumerName: clientId,
		}

		if parent.Spec.HasM2M() {
			routeConsumer.Spec.Security = &gatewayapi.ConsumerSecurity{
				M2M: &gatewayapi.ConsumerMachine2MachineAuthentication{
					Scopes: parent.Spec.Security.M2M.Scopes,
				},
			}
		}

		return nil
	}

	_, err := scopedClient.CreateOrUpdate(ctx, routeConsumer, mutate)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to create ConsumeRoute %s in namespace: %s",
			parent.GetName(), parent.GetNamespace())
	}

	return routeConsumer, nil
}
