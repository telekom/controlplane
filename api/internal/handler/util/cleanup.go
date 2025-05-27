// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// CleanupProxyRoute deletes the route only if no other subscriptions (size > 1) for this route exist
func CleanupProxyRoute(ctx context.Context, routeRef *types.ObjectRef) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	log := log.FromContext(ctx)

	if routeRef == nil {
		return nil
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
