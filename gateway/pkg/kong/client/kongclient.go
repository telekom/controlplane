// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package client turns the controller's desired Kong state into Admin API
// calls, writing only when the desired state differs from what Kong holds.
//
// One file per Kong entity holds its public operations next to the private
// entity type that drives them; reconcile.go, response.go and normalize.go hold
// the machinery those entities share.
package client

import (
	"context"
	"fmt"
	"strings"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

type KongClient interface {
	CreateOrReplaceRoute(ctx context.Context, route CustomRoute, upstream Upstream) error
	DeleteRoute(ctx context.Context, route CustomRoute) error

	CreateOrReplaceConsumer(ctx context.Context, consumer CustomConsumer) (kongConsumer *kong.Consumer, err error)
	DeleteConsumer(ctx context.Context, consumer CustomConsumer) error

	CreateOrReplacePlugin(ctx context.Context, plugin CustomPlugin) (kongPlugin *kong.Plugin, err error)
	DeletePlugin(ctx context.Context, plugin CustomPlugin) error

	CleanupPlugins(ctx context.Context, route CustomRoute, consumer CustomConsumer, plugins []CustomPlugin) error

	CreateOrReplaceUpstream(ctx context.Context, route CustomRoute, upstream *kong.CreateUpstreamJSONRequestBody, target *kong.CreateTargetForUpstreamJSONRequestBody) error
	DeleteUpstream(ctx context.Context, route CustomRoute) error
}

type KongAdminApi interface {
	GetPluginWithResponse(ctx context.Context, pluginId string, reqEditors ...kong.RequestEditorFn) (*kong.GetPluginResponse, error)
	DeletePluginWithResponse(ctx context.Context, pluginId string, reqEditors ...kong.RequestEditorFn) (*kong.DeletePluginResponse, error)
	ListPluginWithResponse(ctx context.Context, params *kong.ListPluginParams, reqEditors ...kong.RequestEditorFn) (*kong.ListPluginResponse, error)
	UpsertPluginWithResponse(ctx context.Context, pluginId string, body kong.UpsertPluginJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertPluginResponse, error)
	UpsertPluginForRouteWithResponse(ctx context.Context, routeIdOrName, pluginId string, body kong.UpsertPluginForRouteJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertPluginForRouteResponse, error)
	UpsertPluginForConsumerWithResponse(ctx context.Context, consumerUsernameOrId, pluginId string, body kong.UpsertPluginForConsumerJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertPluginForConsumerResponse, error)

	UpsertUpstreamWithResponse(ctx context.Context, upstreamIdOrName string, body kong.UpsertUpstreamJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertUpstreamResponse, error)
	GetUpstreamWithResponse(ctx context.Context, upstreamIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.GetUpstreamResponse, error)
	ListTargetsForUpstreamWithResponse(ctx context.Context, upstreamIdOrName string, params *kong.ListTargetsForUpstreamParams, reqEditors ...kong.RequestEditorFn) (*kong.ListTargetsForUpstreamResponse, error)
	CreateTargetForUpstreamWithResponse(ctx context.Context, upstreamIdOrName string, body kong.CreateTargetForUpstreamJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.CreateTargetForUpstreamResponse, error)
	DeleteUpstreamWithResponse(ctx context.Context, upstreamIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteUpstreamResponse, error)
	DeleteUpstreamTargetWithResponse(ctx context.Context, upstreamIdOrName, targetIdOrTarget string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteUpstreamTargetResponse, error)

	UpsertServiceWithResponse(ctx context.Context, serviceIdOrName string, body kong.UpsertServiceJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertServiceResponse, error)
	GetServiceWithResponse(ctx context.Context, serviceIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.GetServiceResponse, error)

	UpsertRouteWithResponse(ctx context.Context, routeIdOrName string, body kong.UpsertRouteJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertRouteResponse, error)
	GetRouteWithResponse(ctx context.Context, routeIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.GetRouteResponse, error)
	DeleteRouteWithResponse(ctx context.Context, routeIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteRouteResponse, error)
	DeleteServiceWithResponse(ctx context.Context, serviceIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteServiceResponse, error)

	UpsertConsumerWithResponse(ctx context.Context, consumerUsernameOrId string, body kong.UpsertConsumerJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertConsumerResponse, error)
	GetConsumerWithResponse(ctx context.Context, consumerUsernameOrId string, reqEditors ...kong.RequestEditorFn) (*kong.GetConsumerResponse, error)

	DeleteConsumerWithResponse(ctx context.Context, consumerUsernameOrId string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteConsumerResponse, error)
	AddConsumerToGroupWithResponse(ctx context.Context, consumerNameOrId string, body kong.AddConsumerToGroupJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.AddConsumerToGroupResponse, error)
	ViewGroupConsumerWithResponse(ctx context.Context, consumerNameOrId string, reqEditors ...kong.RequestEditorFn) (*kong.ViewGroupConsumerResponse, error)
}

var _ KongClient = &kongClient{}

type kongClient struct {
	client KongAdminApi
}

var NewKongClient = func(client KongAdminApi) KongClient {
	return &kongClient{client: client}
}

func BuildTag(key, value string) string {
	return fmt.Sprintf("%s--%s", key, value)
}

func encodeTags(tags []string) *string {
	if len(tags) == 0 {
		return nil
	}
	strTags := strings.Join(tags, ",")
	return &strTags
}

func toPtrOrNil[T any](v []T) *[]T {
	if len(v) == 0 {
		return nil
	}
	return &v
}

func ptrOf[T any](v T) *T {
	return &v
}
