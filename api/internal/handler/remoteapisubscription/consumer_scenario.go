// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remoteapisubscription

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// handleConsumerScenario handles the case where the RemoteApiSubscription is handled by another CP.
// That means that the current CP just needs to create the route to the other CP.
// This route is considered a real-route and is shared for all subscriptions to the same API and target CP.
func (h *RemoteApiSubscriptionHandler) handleConsumerScenario(ctx context.Context, obj *apiapi.RemoteApiSubscription) (err error) {
	logger := log.FromContext(ctx)
	c := client.ClientFromContextOrDie(ctx)

	// Send it to other CP
	logger.V(1).Info("I need to send this to the other CP")
	remoteOrgRef := types.ObjectRef{
		Name:      obj.Spec.TargetOrganization,
		Namespace: contextutil.EnvFromContextOrDie(ctx),
	}
	remoteOrg, err := util.GetRemoteOrganization(ctx, remoteOrgRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get remote organization %s", remoteOrgRef.Name)
	}

	updated, obj, err := h.SyncerFactory.NewClient(remoteOrg).Send(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "failed to send RemoteApiSubscription to remote CP")
	}
	if updated {

		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonProvisioning, "Syncing with remote CP"))
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonProvisioning, "Syncing with remote CP"))
		return nil
	}
	logger.V(1).Info("RemoteApiSubscription synced with remote CP but not updated")

	if !meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady) {
		logger.V(1).Info("RemoteApiSubscription not ready")
		return nil
	}

	// Resolve zone and default preset for hostnames/paths
	preset, zone, err := util.GetDefaultPresetForZone(ctx, remoteOrg.Spec.Zone)
	if err != nil {
		return errors.Wrapf(err, "failed to get default preset for zone %s", remoteOrg.Spec.Zone.Name)
	}
	if zone.Status.Gateway == nil {
		return errors.Errorf("zone %s has no gateway reference in status", remoteOrg.Spec.Zone.Name)
	}

	// Create real route
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteOrg.Spec.Id + "--" + labelutil.NormalizeValue(obj.Spec.ApiBasePath),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			apiapi.BasePathLabelKey:      labelutil.NormalizeLabelValue(obj.Spec.ApiBasePath),
			config.BuildLabelKey("zone"): labelutil.NormalizeValue(zone.Name),
			config.BuildLabelKey("type"): "real",
		}

		u, parseErr := url.Parse(obj.Status.GatewayUrl)
		if parseErr != nil {
			return errors.Wrapf(parseErr, "failed to parse gateway url")
		}

		port := gatewayapi.GetPortOrDefaultFromScheme(u)

		hostnames, paths := preset.ResolveHostnamesAndPaths(obj.Spec.ApiBasePath)

		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayapi.RouteTypePrimary,
			Backend: gatewayapi.Backend{
				Upstreams: []gatewayapi.Upstream{
					{
						Scheme:   u.Scheme,
						Hostname: u.Hostname(),
						Port:     port,
						Path:     u.Path,
					},
				},
			},
			Hostnames: hostnames,
			Paths:     paths,
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create route %s", route.Name)
	}

	obj.Status.Route = types.ObjectRefFromObject(route)

	return nil
}
