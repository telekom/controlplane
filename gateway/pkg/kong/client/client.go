// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"io"
	"net/http"
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
	CreateOrReplaceRoute(ctx context.Context, route CustomRoute, upstream Upstream, gateway *gatewayapi.Gateway) error
	DeleteRoute(ctx context.Context, route CustomRoute) error

	CreateOrReplaceConsumer(ctx context.Context, consumer CustomConsumer) (kongConsumer *kong.Consumer, err error)
	DeleteConsumer(ctx context.Context, consumer CustomConsumer) error

	LoadPlugin(ctx context.Context, plugin CustomPlugin, copyConfig bool) (kongPlugin *kong.Plugin, err error)

	CreateOrReplacePlugin(ctx context.Context, plugin CustomPlugin) (kongPlugin *kong.Plugin, err error)
	DeletePlugin(ctx context.Context, plugin CustomPlugin) error

	CleanupPlugins(ctx context.Context, route CustomRoute, consumer CustomConsumer, plugins []CustomPlugin) error
}

type KongAdminApi interface {
	GetPluginWithResponse(ctx context.Context, pluginId string, reqEditors ...kong.RequestEditorFn) (*kong.GetPluginResponse, error)
	DeletePluginWithResponse(ctx context.Context, pluginId string, reqEditors ...kong.RequestEditorFn) (*kong.DeletePluginResponse, error)
	ListPluginWithResponse(ctx context.Context, params *kong.ListPluginParams, reqEditors ...kong.RequestEditorFn) (*kong.ListPluginResponse, error)

	UpsertUpstreamWithResponse(ctx context.Context, upstreamIdOrName string, body kong.UpsertUpstreamJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertUpstreamResponse, error)
	CreateTargetForUpstreamWithResponse(ctx context.Context, upstreamIdOrName string, body kong.CreateTargetForUpstreamJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.CreateTargetForUpstreamResponse, error)

	UpsertServiceWithResponse(ctx context.Context, serviceIdOrName string, body kong.UpsertServiceJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertServiceResponse, error)

	UpsertRouteWithResponse(ctx context.Context, routeIdOrName string, body kong.UpsertRouteJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertRouteResponse, error)
	DeleteRouteWithResponse(ctx context.Context, routeIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteRouteResponse, error)
	DeleteServiceWithResponse(ctx context.Context, serviceIdOrName string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteServiceResponse, error)

	UpsertConsumerWithResponse(ctx context.Context, consumerUsernameOrId string, body kong.UpsertConsumerJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.UpsertConsumerResponse, error)

	DeleteConsumerWithResponse(ctx context.Context, consumerUsernameOrId string, reqEditors ...kong.RequestEditorFn) (*kong.DeleteConsumerResponse, error)
	AddConsumerToGroupWithResponse(ctx context.Context, consumerNameOrId string, body kong.AddConsumerToGroupJSONRequestBody, reqEditors ...kong.RequestEditorFn) (*kong.AddConsumerToGroupResponse, error)
	ViewGroupConsumerWithResponse(ctx context.Context, consumerNameOrId string, reqEditors ...kong.RequestEditorFn) (*kong.ViewGroupConsumerResponse, error)
}

var _ KongClient = &kongClient{}

type kongClient struct {
	//client     kong.ClientWithResponsesInterface
	client     KongAdminApi
	commonTags []string
}

var NewKongClient = func(client KongAdminApi, commonTags ...string) KongClient {
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
	}

	if plugin.GetRoute() != nil {
		tags = append(tags, buildTag("route", *plugin.GetRoute()))
	}

	if plugin.GetConsumer() != nil {
		tags = append(tags, buildTag("consumer", *plugin.GetConsumer()))
	} else {
		tags = append(tags, buildTag("consumer", "none"))
	}

	if pluginId != "" {
		log.V(1).Info("loading plugin by id", "id", pluginId)
		response, err := c.client.GetPluginWithResponse(ctx, pluginId)
		if err != nil {
			return nil, err
		}
		if err := CheckStatusCode(response, 200, 404); err != nil {
			return nil, fmt.Errorf("failed to get plugin: (%d): %s", response.StatusCode(), string(response.Body))
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

func (c *kongClient) CreateOrReplacePlugin(
	ctx context.Context, plugin CustomPlugin) (kongPlugin *kong.Plugin, err error) {

	log := logr.FromContextOrDiscard(ctx)
	envName := contextutil.EnvFromContextOrDie(ctx)

	isRouteSpecific := plugin.GetRoute() != nil
	isConsumerSpecific := plugin.GetConsumer() != nil

	tags := []string{
		buildTag("env", envName),
		buildTag("plugin", plugin.GetName()),
	}

	if isRouteSpecific {
		tags = append(tags, buildTag("route", *plugin.GetRoute()))
	}

	if isConsumerSpecific {
		tags = append(tags, buildTag("consumer", *plugin.GetConsumer()))
	} else {
		tags = append(tags, buildTag("consumer", "none"))
	}

	kongPlugin, err = c.LoadPlugin(ctx, plugin, false)
	if err != nil {
		return nil, err
	}

	pluginName := plugin.GetName()
	pluginConfig := plugin.GetConfig()
	pluginEnabled := true
	body := kong.UpsertPluginJSONRequestBody{
		Enabled:  &pluginEnabled,
		Name:     &pluginName,
		Config:   &pluginConfig,
		Consumer: nil,
		Service:  nil,
		Route:    nil,
		Protocols: &[]kong.CreatePluginForConsumerRequestProtocols{
			kong.CreatePluginForConsumerRequestProtocolsHttp,
		},
		Tags: &tags,
	}

	if isConsumerSpecific {
		// If the plugin is for a consumer, set the reference to the consumer in the plugin-request.
		body.Consumer = plugin.GetConsumer()
	}

	// If the plugin is for a route or a consumer on a route,
	// set the reference to the route in the plugin-request.
	if isRouteSpecific {
		body.Route = &map[string]any{
			"name": plugin.GetRoute(),
		}
	}

	client, ok := c.client.(kong.ClientInterface)
	if !ok {
		return nil, fmt.Errorf("invalid client type: %T", c.client)
	}

	var pluginId string
	if kongPlugin != nil {
		pluginId = *kongPlugin.Id
	} else {
		pluginId = uuid.NewString()
		log.V(1).Info("generated new plugin id", "id", pluginId, "plugin", plugin.GetName())
	}

	var response *http.Response

	// Order is important here:
	// 1. If a consumer is set on the plugin, then the plugin is created for that consumer.
	// 2. If a route and a consumer are set on the plugin, then the plugin is created for that consumer on that route.
	// 3. If a route is set on the plugin, then the plugin is created for that route.
	if isConsumerSpecific {
		// If a consumer is set on the plugin, then the plugin is created for that consumer.
		// It is also possible to define a route in addition to the consumer.
		// In that case, the plugin is created for the consumer on that route.

		log.V(1).Info("upserting plugin for consumer", "consumer", *plugin.GetConsumer(), "id", pluginId)
		response, err = client.UpsertPluginForConsumer(ctx, *plugin.GetConsumer(), pluginId, body)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create plugin")
		}

	} else if isRouteSpecific {
		// If a route is set on the plugin, then the plugin is created for that route.
		// This means, it is applied for all consumers of that route.

		log.V(1).Info("upserting plugin for route", "route", *plugin.GetRoute(), "id", pluginId)
		response, err = client.UpsertPluginForRoute(ctx, *plugin.GetRoute(), pluginId, body)
		if err != nil {
			return nil, errors.Wrap(err, "failed to upsert plugin for route")
		}

	} else {
		// global plugin
		log.V(1).Info("upserting global plugin", "id", pluginId)
		response, err = client.UpsertPlugin(ctx, pluginId, body)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create plugin")
		}
	}

	apiResponse := WrapApiResponse(response)
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}
	response.Body.Close() //nolint:errcheck

	if err := CheckStatusCode(apiResponse, 200); err != nil {
		return nil, fmt.Errorf("failed to create plugin: (%d): %s", apiResponse.StatusCode(), string(responseBody))
	}
	err = json.Unmarshal(responseBody, &kongPlugin)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal plugin response")
	}

	plugin.SetId(pluginId)
	return kongPlugin, nil
}

func (c *kongClient) DeletePlugin(ctx context.Context, plugin CustomPlugin) (err error) {
	envName := contextutil.EnvFromContextOrDie(ctx)
	pluginId := plugin.GetId()
	tags := []string{
		buildTag("env", envName),
		buildTag("plugin", plugin.GetName()),
	}

	if plugin.GetRoute() == nil && plugin.GetConsumer() == nil {
		return fmt.Errorf("either route or consumer must be provided for deletion")
	}

	if plugin.GetRoute() != nil {
		tags = append(tags, buildTag("route", *plugin.GetRoute()))
	}

	if plugin.GetConsumer() != nil {
		tags = append(tags, buildTag("consumer", *plugin.GetConsumer()))
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
		return fmt.Errorf("failed to delete plugin: (%d): %s", response.StatusCode(), string(response.Body))
	}
	return nil
}

func (c *kongClient) CleanupPlugins(ctx context.Context, route CustomRoute, consumer CustomConsumer, plugins []CustomPlugin) error {
	log := logr.FromContextOrDiscard(ctx)
	envName := contextutil.EnvFromContextOrDie(ctx)
	tags := []string{
		buildTag("env", envName),
	}

	if route == nil && consumer == nil {
		return errors.New("either route or consumer must be provided for cleanup")
	}

	if route != nil {
		tags = append(tags, buildTag("route", route.GetName()))
	}
	if consumer != nil {
		tags = append(tags, buildTag("consumer", consumer.GetConsumerName()))
	}

	kongPlugins, err := c.getPluginsMatchingTags(ctx, tags)
	if err != nil {
		return errors.Wrap(err, "failed to list plugins")
	}

	pluginIds := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		pluginIds = append(pluginIds, plugin.GetId())
	}

	log.Info("cleaning up plugins",
		"found", len(kongPlugins),
		"expected", len(pluginIds),
		"need_cleanup", len(kongPlugins) != len(pluginIds),
	)

	for _, kongPlugin := range kongPlugins {
		if !slices.Contains(pluginIds, *kongPlugin.Id) {
			log.V(1).Info("deleting plugin", "name", *kongPlugin.Name, "id", *kongPlugin.Id)
			_, err := c.client.DeletePluginWithResponse(ctx, *kongPlugin.Id)
			if err != nil {
				return errors.Wrap(err, "failed to delete plugin")
			}
		}
	}

	return nil
}

func (c *kongClient) getPluginsMatchingTags(
	ctx context.Context, tags []string) ([]kong.Plugin, error) {

	// ListPluginsForRouteWithResponse does not work correctly with tags
	response, err := c.client.ListPluginWithResponse(ctx, &kong.ListPluginParams{
		Tags: encodeTags(tags),
	})
	if err != nil {
		return nil, err
	}
	if err := CheckStatusCode(response, 200); err != nil {
		return nil, fmt.Errorf("failed to list plugins: (%d): %s", response.StatusCode(), string(response.Body))
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

	return responseBody.Data, nil
}

func (c *kongClient) getPluginMatchingTags(
	ctx context.Context, tags []string) (*kong.Plugin, error) {

	plugins, err := c.getPluginsMatchingTags(ctx, tags)
	if err != nil {
		return nil, err
	}

	length := len(plugins)

	switch length {
	case 0:
		return nil, nil
	case 1:
		return &plugins[0], nil
	default:
		return nil, fmt.Errorf("found multiple plugins with tags: %s", *encodeTags(tags))
	}
}

func (c *kongClient) CreateOrReplaceRoute(ctx context.Context, route CustomRoute, upstream Upstream, gateway *gatewayapi.Gateway) error {
	if upstream == nil {
		return fmt.Errorf("upstream is required")
	}

	routeName := route.GetName()
	upstreamPath := upstream.GetPath()
	serviceName := routeName
	serviceHost := upstream.GetHost()

	// if CB is enabled (either global config for gateway, or bypass configured directly on Route)
	if isCircuitBreakerEnabled(ctx, gateway, route) {
		upstreamAlgorithm := kong.RoundRobin
		passiveHealthcheckType := kong.CreateUpstreamRequestHealthchecksPassiveTypeHttp
		activeHealthcheckType := kong.CreateUpstreamRequestHealthchecksActiveTypeHttp
		upstreamName := routeName
		upstreamBody := kong.CreateUpstreamJSONRequestBody{
			Algorithm: &upstreamAlgorithm,
			Name:      upstreamName,
			Healthchecks: &kong.CreateUpstreamRequestHealthchecks{
				Active: &kong.CreateUpstreamRequestHealthchecksActive{
					Healthy: &kong.CreateUpstreamRequestHealthchecksActiveHealthy{
						HttpStatuses: &gateway.Spec.CircuitBreaker.Active.HealthyHttpStatuses,
					},
					Type: &activeHealthcheckType,
					Unhealthy: &kong.CreateUpstreamRequestHealthchecksActiveUnhealthy{
						HttpStatuses: &gateway.Spec.CircuitBreaker.Active.UnhealthyHttpStatuses,
					},
				},
				Passive: &kong.CreateUpstreamRequestHealthchecksPassive{
					Healthy: &kong.CreateUpstreamRequestHealthchecksPassiveHealthy{
						HttpStatuses: toPassiveHealthyHttpStatuses(gateway.Spec.CircuitBreaker.Passive.HealthyHttpStatuses),
						Successes:    &gateway.Spec.CircuitBreaker.Passive.HealthySuccesses,
					},
					Type: &passiveHealthcheckType,
					Unhealthy: &kong.CreateUpstreamRequestHealthchecksPassiveUnhealthy{
						HttpFailures: &gateway.Spec.CircuitBreaker.Passive.UnhealthyHttpFailures,
						HttpStatuses: toPassiveUnhealthyHttpStatuses(gateway.Spec.CircuitBreaker.Passive.UnhealthyHttpStatuses),
						TcpFailures:  &gateway.Spec.CircuitBreaker.Passive.UnhealthyTcpFailures,
						Timeouts:     &gateway.Spec.CircuitBreaker.Passive.UnhealthyTimeouts,
					},
				},
			},
			Tags: &[]string{
				buildTag("env", contextutil.EnvFromContextOrDie(ctx)),
				buildTag("upstream", upstreamName),
			},
		}

		upstreamResponse, err := c.client.UpsertUpstreamWithResponse(ctx, upstreamName, upstreamBody)
		if err != nil {
			return errors.Wrap(err, "failed to create upstream")
		}
		if err := CheckStatusCode(upstreamResponse, 200); err != nil {
			return errors.Wrap(fmt.Errorf("failed to create upstream: %s", string(upstreamResponse.Body)), "failed to create upstream")
		}

		// important - the service needs to explicitly use this upstream for circuit breaker to work
		serviceHost = upstreamName

		targetsName := routeName
		targetsTarget := "localhost:8080"
		targetsWeight := 100
		targetsBody := kong.CreateTargetForUpstreamJSONRequestBody{
			Tags: &[]string{
				buildTag("env", contextutil.EnvFromContextOrDie(ctx)),
				buildTag("targets", targetsName),
			},
			Target: &targetsTarget,
			Weight: &targetsWeight,
		}

		// this is a special case with the kong admin API - this endpoint /upstreams/:upstreamName/targets actually accepts multiple POST requests, so this is not a mistake
		targetsResponse, err := c.client.CreateTargetForUpstreamWithResponse(ctx, upstreamName, targetsBody)
		if err != nil {
			return errors.Wrap(err, "failed to create targets for upstream")
		}
		if err := CheckStatusCode(upstreamResponse, 200); err != nil {
			return errors.Wrap(fmt.Errorf("failed to create targets for upstream: %s", string(targetsResponse.Body)), "failed to create targets for upstream")
		}
	}

	serviceBody := kong.CreateServiceJSONRequestBody{
		Enabled:  true,
		Name:     &serviceName,
		Host:     serviceHost,
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

func (c *kongClient) CreateOrReplaceConsumer(ctx context.Context, consumer CustomConsumer) (kongConsumer *kong.Consumer, err error) {
	envName := contextutil.EnvFromContextOrDie(ctx)
	consumerName := consumer.GetConsumerName()
	tags := []string{
		buildTag("env", envName),
		buildTag("consumer", consumerName),
	}

	response, err := c.client.UpsertConsumerWithResponse(ctx, consumerName, kong.CreateConsumerJSONRequestBody{
		CustomId: consumerName,
		Tags:     &tags,
	})
	if err != nil {
		return nil, err
	}
	if err := CheckStatusCode(response, 200); err != nil {
		return nil, fmt.Errorf("failed to create consumer: (%d): %s", response.StatusCode(), string(response.Body))
	}

	isInGroup, err := c.isConsumerInGroup(ctx, consumerName)
	if err != nil {
		return nil, err
	}
	if !isInGroup {
		err = c.addConsumerToGroup(ctx, consumerName)
		if err != nil {
			return nil, errors.Wrap(err, "failed to add consumer to group")
		}
	}

	// The Api-Spec defines a wrong type for the response body, so we need to unmarshal it manually
	err = json.Unmarshal(response.Body, &kongConsumer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consumer response")
	}

	consumer.SetId(*kongConsumer.Id)
	return kongConsumer, nil
}

func (c *kongClient) DeleteConsumer(ctx context.Context, consumer CustomConsumer) error {
	response, err := c.client.DeleteConsumerWithResponse(ctx, consumer.GetConsumerName())
	if err != nil {
		return err
	}
	if err := CheckStatusCode(response, 200, 204, 404); err != nil {
		return fmt.Errorf("failed to delete consumer (%d): %s", response.StatusCode(), string(response.Body))
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
	if err := CheckStatusCode(response, 200, 201); err != nil {
		return fmt.Errorf("failed to add consumer to group (%d): %s", response.StatusCode(), string(response.Body))
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

// isCircuitBreakerEnabled if CB is defined (thus enabled). Its possible to override it via an internal field in the Route.Spec.Traffic.CircuitBreaker
func isCircuitBreakerEnabled(ctx context.Context, gateway *gatewayapi.Gateway, route CustomRoute) bool {
	log := logr.FromContextOrDiscard(ctx)
	// check if the route is trying to bypass the GW config
	r, ok := route.(*gatewayapi.Route)
	if ok {
		log.Info("Cannot convert CustomRoute to gatewayapi.Route when attempting to resolve if CircuitBreaker should be configured. Assuming the value is FALSE!", "routeName", route.GetName())
	}
	if r.Spec.Traffic.CircuitBreaker != nil {
		log.Info("Route has explicitly defined a CircuitBreaker value - bypassing gateway configuration!", "routeName", route.GetName())
		return *r.Spec.Traffic.CircuitBreaker
	}

	if gateway == nil {
		return false
	}
	if gateway.Spec.CircuitBreaker != nil {
		return true
	}
	return false
}

func toPassiveUnhealthyHttpStatuses(statuses []int) *[]kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses {
	result := make([]kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses, len(statuses))
	for i, status := range statuses {
		result[i] = kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses(status)
	}
	return &result
}

func toPassiveHealthyHttpStatuses(statuses []int) *[]kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses {
	result := make([]kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses, len(statuses))
	for i, status := range statuses {
		result[i] = kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses(status)
	}
	return &result
}
