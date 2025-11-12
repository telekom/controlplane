// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package remoteapisubscription

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// handleConsumerScenario handles the case where the RemoteApiSubscription is handled by another CP.
// That means that the current CP just needs to create the route to the other CP.
// This route is considered a real-route and is shared for all subscriptions to the same API and target CP.
func (h *RemoteApiSubscriptionHandler) handleConsumerScenario(ctx context.Context, obj *apiapi.RemoteApiSubscription) (err error) {
	log := log.FromContext(ctx)
	c := client.ClientFromContextOrDie(ctx)

	// Send it to other CP
	log.V(1).Info("I need to send this to the other CP")
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

		obj.SetCondition(condition.NewProcessingCondition("Syncing", "Syncing with remote CP"))
		obj.SetCondition(condition.NewNotReadyCondition("Syncing", "Syncing with remote CP"))
		return nil
	}
	log.V(1).Info("RemoteApiSubscription synced with remote CP but not updated")

	if !meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady) {
		log.V(1).Info("🫷 RemoteApiSubscription not ready")
		return nil
	}

	zone, err := util.GetZone(ctx, c, remoteOrg.Spec.Zone.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get zone %s", remoteOrg.Spec.Zone.Name)
	}

	downstreamRealmRef := types.ObjectRef{
		Name:      remoteOrg.Spec.Id,
		Namespace: zone.Status.Namespace,
	}
	downstreamRealm, err := util.GetRealm(ctx, downstreamRealmRef.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get realm '%s'", downstreamRealmRef.String())
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
			apiapi.BasePathLabelKey:       labelutil.NormalizeLabelValue(obj.Spec.ApiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(zone.Name),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(downstreamRealm.Name),
			config.BuildLabelKey("type"):  "real",
		}

		url, err := url.Parse(obj.Status.GatewayUrl)
		if err != nil {
			return errors.Wrapf(err, "failed to parse gateway url")
		}

		downstream, err := downstreamRealm.AsDownstream(obj.Spec.ApiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm: downstreamRealmRef,
			Upstreams: []gatewayapi.Upstream{
				{
					Scheme: url.Scheme,
					Host:   url.Hostname(),
					Port:   gatewayapi.GetPortOrDefaultFromScheme(url),
					Path:   url.Path,
				},
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
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
