// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"encoding/json"
	"slices"
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
	closeClient     func()
	tags            []string
	listRatelimiter *rate.Limiter
}

func NewKongSource(kongClient kong.ClientWithResponsesInterface) (source *KongSource) {
	return &KongSource{
		kongClient:      kongClient,
		listRatelimiter: rate.NewLimiter(rate.Every(5*time.Second), 20),
	}
}

// NewKongSourceFromConfig creates a source with a client that verifies TLS
// unless cfg.InsecureSkipVerify is set. The client lives until Close is called.
func NewKongSourceFromConfig(cfg config.SourceConfig, tags []string) (source Source, err error) {
	kongClient, closeClient, err := kongutil.NewStandaloneClient(context.Background(), cfg, cfg.InsecureSkipVerify)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kong client from config")
	}
	s := NewKongSource(kongClient)
	s.closeClient = closeClient
	s.environment = cfg.Environment
	s.zone = cfg.Zone
	s.tags = tags
	return s, nil
}

// Close releases the resources of a client created from config.
func (k *KongSource) Close() {
	if k.closeClient != nil {
		k.closeClient()
	}
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

// TakeGlobalSnapshot lists up to limit resources (0 = no limit) and snapshots them asynchronously into ch.
// Listing happens synchronously and its errors are returned; ch is closed when all snapshots are taken.
// The resources are the first N in the gateway's listing order and are processed in sorted order.
func (k *KongSource) TakeGlobalSnapshot(ctx context.Context, resourceType string, limit int, ch chan<- *snapshot.Snapshot) (err error) {
	ids, err := k.listIDs(ctx, resourceType, limit)
	if err != nil {
		return err
	}
	slices.Sort(ids)
	zap.L().Info("selected resources for global snapshot", zap.String("resourceType", resourceType), zap.Int("count", len(ids)), zap.Int("limit", limit))

	go func() {
		defer close(ch)
		for _, resourceId := range ids {
			if ctx.Err() != nil {
				zap.L().Info("stopping snapshot taking due to context done")
				return
			}
			snap, err := k.TakeSnapshot(ctx, resourceType, resourceId)
			if err != nil {
				zap.L().Error("failed to take snapshot for resource", zap.String("resourceType", resourceType), zap.String("id", resourceId), zap.Error(err))
				continue
			}
			select {
			case ch <- snap:
				zap.L().Info("taken snapshot for resource", zap.String("resourceType", resourceType), zap.String("id", resourceId))
			case <-ctx.Done():
				zap.L().Info("stopping snapshot taking due to context done")
				return
			}
		}
	}()

	return nil
}

const maxListPageSize = 100

// pageFunc fetches one page of identifiers with the given size and offset and returns the next offset.
type pageFunc func(ctx context.Context, size int, offset *string) (ids []string, next *string, err error)

func (k *KongSource) listIDs(ctx context.Context, resourceType string, limit int) ([]string, error) {
	switch {
	case strings.EqualFold(resourceType, "route"):
		ids, err := k.listAll(ctx, limit, k.routesPage)
		return ids, errors.Wrap(err, "failed to list routes")
	case strings.EqualFold(resourceType, "consumer"):
		ids, err := k.listAll(ctx, limit, k.consumersPage)
		return ids, errors.Wrap(err, "failed to list consumers")
	default:
		return nil, errors.Errorf("unsupported resource type: %s", resourceType)
	}
}

// listAll follows the offset of page until the listing ends or limit identifiers are collected (0 = no limit).
func (k *KongSource) listAll(ctx context.Context, limit int, page pageFunc) ([]string, error) {
	var ids []string
	var offset *string
	for {
		size := maxListPageSize
		if limit > 0 {
			size = min(limit-len(ids), maxListPageSize)
		}
		if err := k.listRatelimiter.Wait(ctx); err != nil {
			return nil, errors.Wrap(err, "rate limiter wait failed")
		}
		pageIDs, next, err := page(ctx, size, offset)
		if err != nil {
			return nil, err
		}
		ids = append(ids, pageIDs...)
		if limit > 0 && len(ids) >= limit {
			return ids[:limit], nil
		}
		if next == nil || *next == "" {
			return ids, nil
		}
		zap.L().Debug("continue listing", zap.Int("collected", len(ids)))
		offset = next
	}
}

func (k *KongSource) routesPage(ctx context.Context, size int, offset *string) ([]string, *string, error) {
	res, err := k.kongClient.ListRouteWithResponse(ctx, &kong.ListRouteParams{
		Offset: offset,
		Size:   &size,
		Tags:   makeTags(k.tags),
	})
	if err != nil {
		return nil, nil, err
	}
	if !util.Is2xx(res) || res.JSON200 == nil {
		return nil, nil, errors.Errorf("unexpected status code %d", res.StatusCode())
	}
	var ids []string
	if res.JSON200.Data != nil {
		for _, route := range *res.JSON200.Data {
			switch {
			case route.Name != nil:
				ids = append(ids, *route.Name)
			case route.Id != nil:
				ids = append(ids, *route.Id)
			}
		}
	}
	return ids, res.JSON200.Offset, nil
}

func (k *KongSource) consumersPage(ctx context.Context, size int, offset *string) ([]string, *string, error) {
	res, err := k.kongClient.ListConsumerWithResponse(ctx, &kong.ListConsumerParams{
		Offset: offset,
		Size:   &size,
		Tags:   makeTags(k.tags),
	})
	if err != nil {
		return nil, nil, err
	}
	if !util.Is2xx(res) || res.JSON200 == nil {
		return nil, nil, errors.Errorf("unexpected status code %d", res.StatusCode())
	}
	var ids []string
	if res.JSON200.Data != nil {
		for _, consumer := range *res.JSON200.Data {
			if consumer.Id != nil {
				ids = append(ids, *consumer.Id)
			}
		}
	}
	return ids, res.JSON200.Offset, nil
}

func makeTags(tags []string) *string {
	if len(tags) == 0 {
		return nil
	}
	joined := strings.Join(tags, ",")
	return &joined
}
