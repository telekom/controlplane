// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/pkg/errors"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/util"
	"go.uber.org/zap"
)

var _ Source = &KongSource{}

type KongSource struct {
	environment string
	zone        string
	mux         sync.Mutex
	kongClient  kong.ClientWithResponsesInterface
	tags        []string
}

func NewKongSource(kongClient kong.ClientWithResponsesInterface) (source *KongSource) {
	return &KongSource{
		kongClient: kongClient,
	}
}

func NewKongSourceFromConfig(cfg config.SourceConfig, tags []string) (source Source, err error) {
	kongClient, err := kongutil.NewClientFor(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kong client from config")
	}
	s := NewKongSource(kongClient)
	s.environment = cfg.Environment
	s.zone = cfg.Zone
	s.tags = tags
	return s, nil
}

func (k *KongSource) TakeRouteSnapShot(ctx context.Context, routeId string) (snap *snapshot.Snapshot, err error) {
	snap = &snapshot.Snapshot{
		State: &snapshot.State{},
	}
	routeRes, err := k.kongClient.GetRouteWithResponse(ctx, routeId)
	if err != nil {
		return snap, errors.Wrap(err, "failed to get route")
	}
	util.MustBe2xx(routeRes, "Route")
	snap.State.Route = routeRes.JSON200

	serviceRes, err := k.kongClient.GetServiceWithResponse(ctx, *routeRes.JSON200.Service.Id)
	if err != nil {
		return snap, errors.Wrap(err, "failed to get service for route")
	}
	util.MustBe2xx(serviceRes, "Service")
	snap.State.Service = serviceRes.JSON200

	plugins, err := k.kongClient.ListPluginsForRouteWithResponse(ctx, routeId, &kong.ListPluginsForRouteParams{})
	if err != nil {
		return snap, errors.Wrap(err, "failed to list plugins for route")
	}
	util.MustBe2xx(plugins, "Plugins for Route")
	snap.State.Plugins = *plugins.JSON200.Data

	upstream, err := k.kongClient.GetUpstreamWithResponse(ctx, routeId) // per convention, the upstream name is the same as the route name
	if err != nil {
		return snap, errors.Wrap(err, "failed to get upstream for route")
	}
	if util.Is2xx(upstream) { // upstream may not exist for some routes
		snap.State.Upstream = upstream.JSON200

		targets, err := k.kongClient.ListTargetsForUpstreamWithResponse(ctx, *upstream.JSON200.Id, &kong.ListTargetsForUpstreamParams{})
		if err != nil {
			return snap, errors.Wrap(err, "failed to list targets for upstream")
		}
		util.MustBe2xx(targets, "Targets for Upstream")
		snap.State.Targets = *targets.JSON200.Data
	}

	snap.ID = routeId

	return snap, nil
}

func (k *KongSource) TakeConsumerSnapShot(ctx context.Context, consumerId string) (snap *snapshot.Snapshot, err error) {
	snap = &snapshot.Snapshot{
		State: &snapshot.State{},
	}

	consumerRes, err := k.kongClient.GetConsumerWithResponse(ctx, consumerId)
	if err != nil {
		return snap, errors.Wrap(err, "failed to get route")
	}
	util.MustBe2xx(consumerRes, "Consumer")
	snap.State.Consumer = consumerRes.JSON200

	plugins, err := k.kongClient.ListPluginsForConsumerWithResponse(ctx, consumerId, &kong.ListPluginsForConsumerParams{})
	if err != nil {
		return snap, errors.Wrap(err, "failed to list plugins for consumer")
	}
	util.MustBe2xx(plugins, "Plugins for Consumer")
	type PluginsResponse struct {
		Data []kong.Plugin `json:"data"`
	}
	var pluginsResponse PluginsResponse
	err = json.Unmarshal(plugins.Body, &pluginsResponse)
	if err != nil {
		return snap, errors.Wrap(err, "failed to unmarshal plugins response for consumer")
	}
	snap.State.Plugins = append(snap.State.Plugins, pluginsResponse.Data...)
	snap.ID = consumerId
	return snap, nil
}

// TakeSnapshot implements Source.
func (k *KongSource) TakeSnapshot(ctx context.Context, resourceType, routeId string) (snap *snapshot.Snapshot, err error) {
	if strings.EqualFold(resourceType, "route") {
		return k.TakeRouteSnapShot(ctx, routeId)
	} else if strings.EqualFold(resourceType, "consumer") {
		return k.TakeConsumerSnapShot(ctx, routeId)
	} else {
		return snap, errors.Errorf("unsupported resource type: %s", resourceType)
	}
}

func (k *KongSource) TakeGlobalSnapshot(ctx context.Context, resourceType string, limit int) (snap map[string]*snapshot.Snapshot, err error) {
	var ids []string
	if strings.EqualFold(resourceType, "route") {
		ids, err = k.GetRoutes(ctx, limit)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get routes")
		}
	} else if strings.EqualFold(resourceType, "consumer") {
		ids, err = k.GetConsumers(ctx, limit)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get consumers")
		}
	} else {
		return nil, errors.Errorf("unsupported resource type: %s", resourceType)
	}

	zap.L().Info("taking snapshots for resources", zap.String("resourceType", resourceType), zap.Int("count", len(ids)))
	snap = make(map[string]*snapshot.Snapshot)
	for _, routeId := range ids {
		routeSnap, err := k.TakeRouteSnapShot(ctx, routeId)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to take snapshot for route %s", routeId)
		}
		snap[routeId] = routeSnap
	}

	return snap, nil
}

func (k *KongSource) GetConsumers(ctx context.Context, limit int) ([]string, error) {
	consumers, err := k.kongClient.ListConsumerWithResponse(ctx, &kong.ListConsumerParams{
		Offset: nil,
		Size:   &limit,
		Tags:   makeTags(k.tags),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list consumers")
	}
	util.MustBe2xx(consumers, "Consumers")
	consumerNames := make([]string, 0, len(*consumers.JSON200.Data))
	for _, consumer := range *consumers.JSON200.Data {
		consumerNames = append(consumerNames, *consumer.Id)
	}

	return consumerNames, nil
}

func (k *KongSource) GetRoutes(ctx context.Context, limit int) ([]string, error) {
	routes, err := k.kongClient.ListRouteWithResponse(ctx, &kong.ListRouteParams{
		Offset: nil,
		Size:   &limit,
		Tags:   makeTags(k.tags),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list routes")
	}
	util.MustBe2xx(routes, "Routes")
	routeNames := make([]string, 0, len(*routes.JSON200.Data))
	for _, route := range *routes.JSON200.Data {
		routeNames = append(routeNames, *route.Name)
	}

	return routeNames, nil
}

func makeTags(tags []string) *string {
	if len(tags) == 0 {
		return nil
	}
	joined := strings.Join(tags, ",")
	return &joined
}
