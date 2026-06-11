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

		obj.SetCondition(condition.NewProcessingCondition("Syncing", "Syncing with remote CP"))
		obj.SetCondition(condition.NewNotReadyCondition("Syncing", "Syncing with remote CP"))
		return nil
	}
	logger.V(1).Info("RemoteApiSubscription synced with remote CP but not updated")

	if !meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady) {
		logger.V(1).Info("🫷 RemoteApiSubscription not ready")
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

		parsedURL, parseErr := url.Parse(obj.Status.GatewayUrl)
		if parseErr != nil {
			return errors.Wrapf(parseErr, "failed to parse gateway url")
		}

		downstream, downstreamErr := downstreamRealm.AsDownstream(obj.Spec.ApiBasePath)
		if downstreamErr != nil {
			return errors.Wrap(downstreamErr, "failed to create downstream")
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm: downstreamRealmRef,
			Upstreams: []gatewayapi.Upstream{
				{
					Scheme: parsedURL.Scheme,
					Host:   parsedURL.Hostname(),
					Port:   gatewayapi.GetPortOrDefaultFromScheme(parsedURL),
					Path:   parsedURL.Path,
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
