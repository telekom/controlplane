// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/util"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

var _ Source = &KongSource{}

type KongSource struct {
	environment     string
	zone            string
	mux             sync.Mutex
	kongClient      kong.ClientWithResponsesInterface
	tags            []string
	listRatelimiter *rate.Limiter
}

func NewKongSource(kongClient kong.ClientWithResponsesInterface) (source *KongSource) {
	return &KongSource{
		kongClient:      kongClient,
		listRatelimiter: rate.NewLimiter(rate.Every(5*time.Second), 20),
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

	snap.Id = routeId

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
	snap.Id = consumerId
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

func (k *KongSource) TakeGlobalSnapshot(ctx context.Context, resourceType string, limit int, ch chan<- *snapshot.Snapshot) (err error) {
	idsCh := make(chan string, limit)

	if strings.EqualFold(resourceType, "route") {
		err = k.listRoutes(ctx, limit, nil, idsCh)
		if err != nil {
			return errors.Wrap(err, "failed to get routes")
		}
	} else if strings.EqualFold(resourceType, "consumer") {
		err = k.listConsumers(ctx, limit, nil, idsCh)
		if err != nil {
			return errors.Wrap(err, "failed to get consumers")
		}
	} else {
		return errors.Errorf("unsupported resource type: %s", resourceType)
	}

	go func() {
		zap.L().Info("taking snapshots for resources", zap.String("resourceType", resourceType))
		defer close(ch)
		for {
			select {
			case resourceId, ok := <-idsCh:
				if !ok {
					return
				}
				routeSnap, err := k.TakeRouteSnapShot(ctx, resourceId)
				if err != nil {
					zap.L().Error("failed to take snapshot for resource", zap.String("resourceType", resourceType), zap.String("id", resourceId), zap.Error(err))
					continue
				}
				ch <- routeSnap
				zap.L().Info("taken snapshot for resource", zap.String("resourceType", resourceType), zap.String("id", resourceId))
			case <-ctx.Done():
				zap.L().Info("stopping snapshot taking due to context done")
				return
			}
		}
	}()

	return nil
}

func (k *KongSource) listRoutes(ctx context.Context, limit int, offset *string, ch chan<- string) error {
	if err := k.listRatelimiter.Wait(ctx); err != nil {
		return errors.Wrap(err, "rate limiter wait failed")
	}

	pageSize := min(limit, 100)
	routes, err := k.kongClient.ListRouteWithResponse(ctx, &kong.ListRouteParams{
		Offset: offset,
		Size:   &pageSize,
		Tags:   makeTags(k.tags),
	})
	if err != nil {
		return errors.Wrap(err, "failed to list routes")
	}
	util.MustBe2xx(routes, "Routes")
	for _, route := range *routes.JSON200.Data {
		ch <- *route.Name
	}
	nextLimit := limit - len(*routes.JSON200.Data)
	if routes.JSON200.Offset == nil || len(*routes.JSON200.Data) < limit || nextLimit <= 0 {
		close(ch)
		return nil
	}
	zap.L().Debug("continue listing routes", zap.Int("next_limit", nextLimit))
	return k.listRoutes(ctx, nextLimit, routes.JSON200.Offset, ch)
}

func (k *KongSource) listConsumers(ctx context.Context, limit int, offset *string, ch chan<- string) error {
	if err := k.listRatelimiter.Wait(ctx); err != nil {
		return errors.Wrap(err, "rate limiter wait failed")
	}

	consumers, err := k.kongClient.ListConsumerWithResponse(ctx, &kong.ListConsumerParams{
		Offset: offset,
		Size:   &limit,
		Tags:   makeTags(k.tags),
	})
	if err != nil {
		return errors.Wrap(err, "failed to list consumers")
	}
	util.MustBe2xx(consumers, "Consumers")
	for _, consumer := range *consumers.JSON200.Data {
		ch <- *consumer.Id
	}
	if consumers.JSON200.Offset == nil || len(*consumers.JSON200.Data) < limit {
		close(ch)
		return nil
	}
	zap.L().Debug("continue listing consumers", zap.Int("next_limit", limit-len(*consumers.JSON200.Data)))
	return k.listConsumers(ctx, limit-len(*consumers.JSON200.Data), consumers.JSON200.Offset, ch)
}

func makeTags(tags []string) *string {
	if len(tags) == 0 {
		return nil
	}
	joined := strings.Join(tags, ",")
	return &joined
}
