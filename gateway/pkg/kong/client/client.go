// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

type MutatorFunc[T any] func(T) (T, error)

//go:generate mockgen -source=client.go -destination=mock/client.gen.go -package=mock
type KongClient interface {
	CreateOrReplaceRoute(ctx context.Context, route CustomRoute, upstream Upstream) error
	DeleteRoute(ctx context.Context, route CustomRoute) error

	CreateOrReplaceConsumer(ctx context.Context, consumerName string) (err error)
	DeleteConsumer(ctx context.Context, consumerName string) error

	LoadPlugin(ctx context.Context, plugin CustomPlugin, copyConfig bool) (kongPlugin *kong.Plugin, err error)
	// LoadPlugins loads all plugins for the given route and copies the config into the provided plugins
	// It is mandatory that all plugins have the same route
	// If rmSuperfluousPlugins is true, plugins that are not in the provided list will be deleted
	LoadPlugins(ctx context.Context, plugin []CustomPlugin, copyConfig bool, rmSuperfluousPlugins bool) (err error)
	CreateOrReplacePlugin(ctx context.Context, plugin CustomPlugin) (kongPlugin *kong.Plugin, err error)
	DeletePlugin(ctx context.Context, plugin CustomPlugin) error

	CleanupPlugins(ctx context.Context, route CustomRoute, plugins []CustomPlugin) error
}

var _ KongClient = &kongClient{}

type kongClient struct {
	client     kong.ClientWithResponsesInterface
	commonTags []string
}

var NewKongClient = func(client kong.ClientWithResponsesInterface, commonTags ...string) KongClient {
	return &kongClient{
		client:     client,
		commonTags: commonTags,
	}
}

func (c *kongClient) LoadPlugin(
	ctx context.Context, plugin CustomPlugin, copyConfig bool) (kongPlugin *kong.Plugin, err error) {

	log := logr.FromContextOrDiscard(ctx).WithValues("plugin", plugin.GetName())
	pluginId := plugin.GetId()
	envName := contextutil.EnvFromContextOrDie(ctx)
	tags := []string{
		buildTag("env", envName),
		buildTag("plugin", plugin.GetName()),
		buildTag("route", *plugin.GetRoute()),
	}

	if plugin.GetConsumer() != nil {
		tags = append(tags, buildTag("consumer", *plugin.GetConsumer()))
	}

	if pluginId != "" {
		log.V(1).Info("loading plugin by id", "id", pluginId)
		response, err := c.client.GetPluginWithResponse(ctx, pluginId)
		if err != nil {
			return nil, err
		}
		if err := CheckStatusCode(response, 200, 404); err != nil {
			return nil, fmt.Errorf("failed to get plugin: %s", string(response.Body))
		}
		if response.StatusCode() == 404 {
			log.V(1).Info("plugin not found", "id", pluginId)
			goto loadByTags
		}

		if copyConfig {
			err = json.Unmarshal(response.Body, &plugin)
			if err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal plugin response")
			}
		}

		kongPlugin = response.JSON200
		pluginId = *kongPlugin.Id
		plugin.SetId(pluginId)
		return kongPlugin, nil
	}

loadByTags:
	log.V(1).Info("loading plugin by tags", "tags", tags)
	kongPlugin, err = c.getPluginMatchingTags(ctx, tags)
	if err != nil {
		return nil, err
	}

	if kongPlugin != nil {
		log.V(1).Info("found plugin", "id", *kongPlugin.Id)
		pluginId = *kongPlugin.Id
		if copyConfig {
			err = deepCopy(kongPlugin, plugin)
			if err != nil {
				return nil, errors.Wrap(err, "failed to copy plugin config")
			}
		}
	}
	plugin.SetId(pluginId)
	return kongPlugin, nil
}

func (c *kongClient) LoadPlugins(
	ctx context.Context, plugins []CustomPlugin, copyConfig bool, rmSuperfluousPlugins bool) (err error) {

	tags := []string{
		buildTag("env", contextutil.EnvFromContextOrDie(ctx)),
	}

	if len(plugins) == 0 {
		return fmt.Errorf("no plugins provided")
	}

	pluginMap := make(map[string]CustomPlugin)
	routeIdOrName := *plugins[0].GetRoute()
	for _, plugin := range plugins {
		if *plugin.GetRoute() != routeIdOrName {
			return fmt.Errorf("all plugins must have the same route")
		}
		pluginMap[plugin.GetName()] = plugin
	}

	foundPlugins, err := c.client.ListPluginsForRouteWithResponse(ctx, routeIdOrName, &kong.ListPluginsForRouteParams{
		Tags: encodeTags(tags),
	})

	if err != nil {
		return errors.Wrap(err, "failed to list plugins")
	}
	if err := CheckStatusCode(foundPlugins, 200); err != nil {
		return fmt.Errorf("failed to list plugins: %s", string(foundPlugins.Body))
	}

	if len(*foundPlugins.JSON200.Data) == 0 {
		return nil
	}

	for _, foundPlugin := range *foundPlugins.JSON200.Data {
		plugin, ok := pluginMap[*foundPlugin.Name]
		if !ok {
			if rmSuperfluousPlugins {
				response, err := c.client.DeletePluginWithResponse(ctx, *foundPlugin.Id)
				if err != nil {
					return errors.Wrap(err, "failed to delete plugin")
				}

				if err := CheckStatusCode(response, 200, 204, 404); err != nil {
					return fmt.Errorf("failed to delete plugin: %s", string(response.Body))
				}
			}
			continue
		}

		if copyConfig {
			err = deepCopy(foundPlugin, plugin)
			if err != nil {
				return errors.Wrap(err, "failed to copy plugin config")
			}
		}
		plugin.SetId(*foundPlugin.Id)
	}

	return nil
}

func (c *kongClient) CreateOrReplacePlugin(
	ctx context.Context, plugin CustomPlugin) (kongPlugin *kong.Plugin, err error) {

	log := logr.FromContextOrDiscard(ctx)
	envName := contextutil.EnvFromContextOrDie(ctx)
	tags := []string{
		buildTag("env", envName),
		buildTag("plugin", plugin.GetName()),
		buildTag("route", *plugin.GetRoute()),
	}

	kongPlugin, err = c.LoadPlugin(ctx, plugin, false)
	if err != nil {
		return nil, err
	}

	var pluginId string
	if kongPlugin != nil {
		pluginId = *kongPlugin.Id
	} else {
		pluginId = uuid.NewString()
		log.V(1).Info("generated new plugin id", "id", pluginId, "plugin", plugin.GetName())
	}

	pluginName := plugin.GetName()
	pluginConfig := plugin.GetConfig()
	pluginEnabled := true
	body := kong.CreatePluginJSONRequestBody{
		Enabled:  &pluginEnabled,
		Name:     &pluginName,
		Config:   &pluginConfig,
		Consumer: plugin.GetConsumer(),
		Route:    plugin.GetRoute(),
		Service:  nil,
		Protocols: &[]kong.CreatePluginForConsumerRequestProtocols{
			kong.CreatePluginForConsumerRequestProtocolsHttp,
		},
		Tags: &tags,
	}
	routeName := plugin.GetRoute()
	if routeName == nil {
		return nil, fmt.Errorf("route name is required for creating a plugin")
	}
	response, err := c.client.UpsertPluginForRouteWithResponse(ctx, *routeName, pluginId, body)
	if err != nil {
		return nil, err
	}

	if err := CheckStatusCode(response, 200); err != nil {
		return nil, fmt.Errorf("failed to create plugin: %s", string(response.Body))
	}

	plugin.SetId(pluginId)
	return response.JSON200, nil
}

func (c *kongClient) DeletePlugin(ctx context.Context, plugin CustomPlugin) (err error) {
	envName := contextutil.EnvFromContextOrDie(ctx)
	pluginId := plugin.GetId()
	tags := []string{
		buildTag("env", envName),
	}

	if pluginId == "" {
		kongPlugin, err := c.getPluginMatchingTags(ctx, tags)
		if err != nil {
			return err
		}
		if kongPlugin == nil {
			// NOT FOUND
			return nil
		}
		pluginId = *kongPlugin.Id
	}

	response, err := c.client.DeletePluginWithResponse(ctx, pluginId)
	if err != nil {
		return err
	}
	if err := CheckStatusCode(response, 200, 204); err != nil {
		return fmt.Errorf("failed to delete plugin: %s", string(response.Body))
	}
	return nil
}

func (c *kongClient) CleanupPlugins(ctx context.Context, route CustomRoute, plugins []CustomPlugin) error {
	if len(plugins) == 0 {
		return nil
	}
	log := logr.FromContextOrDiscard(ctx).WithValues("route", route.GetName())
	envName := contextutil.EnvFromContextOrDie(ctx)
	tags := []string{
		buildTag("env", envName),
	}

	response, err := c.client.ListPluginsForRouteWithResponse(ctx, route.GetName(), &kong.ListPluginsForRouteParams{
		Tags: encodeTags(tags),
	})

	if err != nil {
		return errors.Wrap(err, "failed to list plugins")
	}
	if err := CheckStatusCode(response, 200); err != nil {
		return fmt.Errorf("failed to list plugins: %s", string(response.Body))
	}

	pluginIds := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		pluginIds = append(pluginIds, plugin.GetId())
	}

	for _, plugin := range *response.JSON200.Data {
		if !slices.Contains(pluginIds, *plugin.Id) {
			log.V(1).Info("deleting plugin", "name", *plugin.Name, "id", *plugin.Id)
			_, err := c.client.DeletePluginWithResponse(ctx, *plugin.Id)
			if err != nil {
				return errors.Wrap(err, "failed to delete plugin")
			}
		}
	}

	return nil
}

func (c *kongClient) getPluginMatchingTags(
	ctx context.Context, tags []string) (*kong.Plugin, error) {

	// ListPluginsForRouteWithResponse does not work correctly with tags
	response, err := c.client.ListPluginWithResponse(ctx, &kong.ListPluginParams{
		Tags: encodeTags(tags),
	})
	if err != nil {
		return nil, err
	}
	if err := CheckStatusCode(response, 200); err != nil {
		return nil, fmt.Errorf("failed to list plugins: %s", string(response.Body))
	}

	// ListPluginWithResponse does not return an array of plugins
	type ResponseBody struct {
		Data []kong.Plugin `json:"data"`
	}
	var responseBody ResponseBody

	err = json.Unmarshal(response.Body, &responseBody)
	if err != nil {
		return nil, err
	}

	length := len(responseBody.Data)

	switch length {
	case 0:
		return nil, nil
	case 1:
		return &responseBody.Data[0], nil
	default:
		return nil, fmt.Errorf("found multiple plugins with tags: %s", *encodeTags(tags))
	}
}

func (c *kongClient) CreateOrReplaceRoute(ctx context.Context, route CustomRoute, upstream Upstream) error {
	if upstream == nil {
		return fmt.Errorf("upstream is required")
	}

	routeName := route.GetName()
	upstreamPath := upstream.GetPath()
	serviceBody := kong.CreateServiceJSONRequestBody{
		Enabled:  true,
		Name:     &routeName,
		Host:     upstream.GetHost(),
		Path:     &upstreamPath,
		Protocol: kong.CreateServiceRequestProtocol(upstream.GetScheme()),
		Port:     upstream.GetPort(),

		Tags: &[]string{
			buildTag("env", contextutil.EnvFromContextOrDie(ctx)),
			buildTag("route", route.GetName()),
		},
	}
	serviceResponse, err := c.client.UpsertServiceWithResponse(ctx, route.GetName(), serviceBody)
	if err != nil {
		return errors.Wrap(err, "failed to create service")
	}
	if err := CheckStatusCode(serviceResponse, 200); err != nil {
		return errors.Wrap(fmt.Errorf("failed to create service: %s", string(serviceResponse.Body)), "failed to create service")
	}

	service := serviceResponse.JSON200
	route.SetServiceId(*service.Id)

	routeBody := kong.CreateRouteJSONRequestBody{
		Name: &routeName,
		Protocols: []string{
			"http",
			"https",
		},
		Paths: &[]string{
			route.GetPath(),
		},
		Hosts: &[]string{
			route.GetHost(),
		},
		Service: &kong.CreateRouteRequestService{
			Id: service.Id,
		},
		RequestBuffering:        true,
		ResponseBuffering:       true,
		HttpsRedirectStatusCode: 426,

		Tags: &[]string{
			buildTag("env", contextutil.EnvFromContextOrDie(ctx)),
			buildTag("route", route.GetName()),
		},
	}
	routeResponse, err := c.client.UpsertRouteWithResponse(ctx, route.GetName(), routeBody)
	if err != nil {
		return errors.Wrap(err, "failed to create route")
	}
	if err := CheckStatusCode(routeResponse, 200); err != nil {
		return errors.Wrap(fmt.Errorf("failed to create route: %s", string(routeResponse.Body)), "failed to create route")
	}

	route.SetRouteId(*routeResponse.JSON200.Id)

	return nil
}

func (c *kongClient) DeleteRoute(ctx context.Context, route CustomRoute) error {
	routeName := route.GetName()
	routeResponse, err := c.client.DeleteRouteWithResponse(ctx, routeName)
	if err != nil {
		return err
	}
	if err := CheckStatusCode(routeResponse, 200, 204, 404); err != nil {
		return fmt.Errorf("failed to delete route: %s", string(routeResponse.Body))
	}

	serviceResponse, err := c.client.DeleteServiceWithResponse(ctx, routeName)
	if err != nil {
		return err
	}
	if err := CheckStatusCode(serviceResponse, 200, 204, 404); err != nil {
		return fmt.Errorf("failed to delete service: %s", string(serviceResponse.Body))
	}

	return nil
}

func (c *kongClient) CreateOrReplaceConsumer(ctx context.Context, consumerName string) (err error) {
	envName := contextutil.EnvFromContextOrDie(ctx)
	tags := []string{
		buildTag("env", envName),
		buildTag("consumer", consumerName),
	}

	response, err := c.client.UpsertConsumerWithResponse(ctx, consumerName, kong.CreateConsumerJSONRequestBody{
		CustomId: consumerName,
		Tags:     &tags,
	})
	if err != nil {
		return err
	}
	if err := CheckStatusCode(response, 200); err != nil {
		return fmt.Errorf("failed to create consumer: %s", string(response.Body))
	}

	isInGroup, err := c.isConsumerInGroup(ctx, consumerName)
	if err != nil {
		return err
	}
	if !isInGroup {
		err = c.addConsumerToGroup(ctx, consumerName)
		if err != nil {
			return errors.Wrap(err, "failed to add consumer to group")
		}
	}

	return nil
}

func (c *kongClient) DeleteConsumer(ctx context.Context, consumerName string) error {
	response, err := c.client.DeleteConsumerWithResponse(ctx, consumerName)
	if err != nil {
		return err
	}
	if err := CheckStatusCode(response, 200, 204, 404); err != nil {
		return fmt.Errorf("failed to delete consumer: %s", string(response.Body))
	}
	return nil
}

func (c *kongClient) addConsumerToGroup(ctx context.Context, consumerName string) error {
	groupName := consumerName
	response, err := c.client.AddConsumerToGroupWithResponse(ctx, consumerName, kong.AddConsumerToGroupJSONRequestBody{
		Group: &groupName,
	})
	if err != nil {
		return err
	}
	if err := CheckStatusCode(response, 200); err != nil {
		return fmt.Errorf("failed to add consumer to group: %s", string(response.Body))
	}

	return nil
}

func (c *kongClient) isConsumerInGroup(ctx context.Context, consumerName string) (bool, error) {
	response, err := c.client.ViewGroupConsumerWithResponse(ctx, consumerName)
	if err != nil {
		return false, errors.Wrap(err, "error occurred when getting consumer group")
	}

	if err := CheckStatusCode(response, 200); err != nil {
		return false, errors.Wrap(err, "error occurred when getting consumer group")
	}

	if len(*response.JSON200.Data) == 0 {
		return false, nil
	} else {
		return true, nil
	}
}

func buildTag(key, value string) string {
	return fmt.Sprintf("%s--%s", key, value)
}

func deepCopy[T any](v any, t T) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &t)
}

func encodeTags(tags []string) *string {
	if len(tags) == 0 {
		return nil
	}
	strTags := strings.Join(tags, ",")
	return &strTags
}
