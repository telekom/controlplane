// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

func (c *kongClient) CreateOrReplacePlugin(ctx context.Context, plugin CustomPlugin) (*kong.Plugin, error) {
	pluginName := plugin.GetName()
	pluginEnabled := true

	config, err := normalizeConfig(plugin.GetConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to normalize desired plugin config: %w", err)
	}

	desired := kong.UpsertPluginJSONRequestBody{
		Enabled:   &pluginEnabled,
		Name:      &pluginName,
		Protocols: convertStringSet[kong.CreatePluginForConsumerRequestProtocols](&[]string{"http"}),
		Tags:      normalizeSet(ptrOf(pluginTags(ctx, plugin))),
	}
	if config != nil {
		desired.Config = &config
	}

	// A consumer-specific plugin references its consumer; a plugin bound to a
	// route references the route, whether or not a consumer is also set.
	if plugin.GetConsumer() != nil {
		desired.Consumer = plugin.GetConsumer()
	}
	if plugin.GetRoute() != nil {
		desired.Route = &map[string]any{"name": plugin.GetRoute()}
	}

	kongPlugin, _, err := reconcile(ctx, &pluginEntity{client: c.client, plugin: plugin}, desired)
	if err != nil {
		return nil, err
	}
	if kongPlugin.Id == nil {
		return nil, fmt.Errorf("plugin response ID is missing")
	}
	plugin.SetId(*kongPlugin.Id)
	return kongPlugin, nil
}

func (c *kongClient) DeletePlugin(ctx context.Context, plugin CustomPlugin) error {
	if plugin.GetRoute() == nil && plugin.GetConsumer() == nil {
		return fmt.Errorf("either route or consumer must be provided for deletion")
	}

	pluginId := plugin.GetId()
	if pluginId == "" {
		kongPlugin, err := getPluginMatchingTags(ctx, c.client, pluginTags(ctx, plugin))
		if err != nil {
			return err
		}
		if kongPlugin == nil {
			return nil
		}
		pluginId = *kongPlugin.Id
	}

	response, err := c.client.DeletePluginWithResponse(ctx, pluginId)
	if err != nil {
		return HandleClientError(err)
	}
	if err := CheckStatusCode(response, http.StatusOK, http.StatusNoContent); err != nil {
		return fmt.Errorf("failed to delete plugin (%d): %s: %w", response.StatusCode(), summarizeBody(response.Body), err)
	}
	return nil
}

func (c *kongClient) CleanupPlugins(ctx context.Context, route CustomRoute, consumer CustomConsumer, plugins []CustomPlugin) error {
	log := logr.FromContextOrDiscard(ctx)

	if route == nil && consumer == nil {
		return errors.New("either route or consumer must be provided for cleanup")
	}

	tags := []string{
		BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
	}
	if route != nil {
		tags = append(tags, BuildTag("route", route.GetName()))
	}
	if consumer != nil {
		tags = append(tags, BuildTag("consumer", consumer.GetConsumerName()))
	}

	kongPlugins, err := getPluginsMatchingTags(ctx, c.client, tags)
	if err != nil {
		return fmt.Errorf("failed to list plugins: %w", err)
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
			response, err := c.client.DeletePluginWithResponse(ctx, *kongPlugin.Id)
			if err != nil {
				return fmt.Errorf("failed to delete plugin: %w", HandleClientError(err))
			}
			if err := CheckStatusCode(response, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
				return fmt.Errorf("failed to delete plugin (%d): %s: %w", response.StatusCode(), summarizeBody(response.Body), err)
			}
		}
	}

	return nil
}

func loadPlugin(ctx context.Context, api KongAdminApi, plugin CustomPlugin) (*kong.Plugin, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("plugin", plugin.GetName())
	pluginId := plugin.GetId()

	if pluginId != "" {
		log.V(1).Info("loading plugin by id", "id", pluginId)
		current, err := loadPluginById(ctx, api, plugin, pluginId)
		if err != nil {
			return nil, err
		}
		if current != nil {
			return current, nil
		}
		log.V(1).Info("plugin not found", "id", pluginId)
	}

	tags := pluginTags(ctx, plugin)
	log.V(1).Info("loading plugin by tags", "tags", tags)
	current, err := getPluginMatchingTags(ctx, api, tags)
	if err != nil {
		return nil, err
	}

	if current != nil {
		if current.Id == nil {
			return nil, fmt.Errorf("plugin response ID is missing")
		}
		log.V(1).Info("found plugin", "id", *current.Id)
		pluginId = *current.Id
	}
	plugin.SetId(pluginId)
	return current, nil
}

// loadPluginById returns nil without an error when Kong does not know the id.
func loadPluginById(ctx context.Context, api KongAdminApi, plugin CustomPlugin, pluginId string) (*kong.Plugin, error) {
	response, err := api.GetPluginWithResponse(ctx, pluginId)
	if err != nil {
		return nil, HandleClientError(err)
	}
	current, found, err := readOne("plugin", readResult[kong.Plugin]{response.StatusCode(), response.Body, response.JSON200})
	if err != nil || !found {
		return nil, err
	}
	if current.Id == nil {
		return nil, fmt.Errorf("plugin response ID is missing")
	}
	plugin.SetId(*current.Id)
	return current, nil
}

// pluginTags builds the ownership tags of a plugin. Lookups tag a plugin
// without a consumer explicitly, so that a route-wide plugin is not confused
// with a consumer-specific one.
func pluginTags(ctx context.Context, plugin CustomPlugin) []string {
	tags := []string{
		BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
		BuildTag("plugin", plugin.GetName()),
	}
	if plugin.GetRoute() != nil {
		tags = append(tags, BuildTag("route", *plugin.GetRoute()))
	}
	if plugin.GetConsumer() != nil {
		tags = append(tags, BuildTag("consumer", *plugin.GetConsumer()))
	} else {
		tags = append(tags, BuildTag("consumer", "none"))
	}
	return tags
}

func getPluginsMatchingTags(ctx context.Context, api KongAdminApi, tags []string) ([]kong.Plugin, error) {
	// ListPluginsForRouteWithResponse does not work correctly with tags
	response, err := api.ListPluginWithResponse(ctx, &kong.ListPluginParams{
		Tags: encodeTags(tags),
	})
	if err != nil {
		return nil, HandleClientError(err)
	}
	if err := CheckStatusCode(response, http.StatusOK); err != nil {
		return nil, fmt.Errorf("failed to list plugins (%d): %s: %w", response.StatusCode(), summarizeBody(response.Body), err)
	}

	// ListPluginWithResponse does not return an array of plugins
	var responseBody struct {
		Data []kong.Plugin `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &responseBody); err != nil {
		return nil, err
	}
	return responseBody.Data, nil
}

func getPluginMatchingTags(ctx context.Context, api KongAdminApi, tags []string) (*kong.Plugin, error) {
	plugins, err := getPluginsMatchingTags(ctx, api, tags)
	if err != nil {
		return nil, err
	}

	switch len(plugins) {
	case 0:
		return nil, nil
	case 1:
		return &plugins[0], nil
	default:
		return nil, fmt.Errorf("found multiple plugins with tags: %s", *encodeTags(tags))
	}
}

type pluginEntity struct {
	client  KongAdminApi
	plugin  CustomPlugin
	current *kong.Plugin
}

var (
	_ entity[kong.UpsertPluginJSONRequestBody, kong.Plugin] = &pluginEntity{}
	_ comparer[kong.UpsertPluginJSONRequestBody]            = &pluginEntity{}
)

func (e *pluginEntity) Name() string { return "plugin" }

// Get reuses the existing lookup, which resolves the plugin by its stored id
// and falls back to the ownership tags.
func (e *pluginEntity) Get(ctx context.Context) (*kong.Plugin, bool, error) {
	current, err := loadPlugin(ctx, e.client, e.plugin)
	if err != nil {
		return nil, false, err
	}
	e.current = current
	return current, current != nil, nil
}

func (e *pluginEntity) Project(current *kong.Plugin) (kong.UpsertPluginJSONRequestBody, error) {
	// normalizeConfig copies before sorting, so the loaded plugin - which is
	// also returned to the caller as the reconciliation result - is untouched.
	config, err := normalizeConfig(valueOrZero(current.Config))
	if err != nil {
		return kong.UpsertPluginJSONRequestBody{}, fmt.Errorf("failed to normalize current plugin config: %w", err)
	}
	projected := kong.UpsertPluginJSONRequestBody{
		Enabled:      current.Enabled,
		InstanceName: current.InstanceName,
		Name:         current.Name,
		Protocols:    convertStringSet[kong.CreatePluginForConsumerRequestProtocols](current.Protocols),
		Tags:         normalizeSet(current.Tags),
	}
	if config != nil {
		projected.Config = &config
	}
	return projected, nil
}

// Equal ignores the entity references. The desired body names the route and the
// consumer while Kong reports their ids, and resolving the names would cost two
// extra reads per plugin. The ownership tags, which are compared, already carry
// the same association.
//
// The configuration is compared only over the keys the desired body names,
// because Kong reports the whole schema including the defaults for everything a
// feature leaves unset.
func (e *pluginEntity) Equal(desired, current kong.UpsertPluginJSONRequestBody) bool {
	desired.Consumer, desired.Route, desired.Service = nil, nil, nil
	current.Consumer, current.Route, current.Service = nil, nil, nil

	desiredConfig := valueOrZero(desired.Config)
	narrowed := narrowToDesired(desiredConfig, valueOrZero(current.Config))
	desired.Config, current.Config = &desiredConfig, &narrowed

	return reflect.DeepEqual(desired, current)
}

func (e *pluginEntity) Write(ctx context.Context, desired *kong.UpsertPluginJSONRequestBody) (*kong.Plugin, error) {
	log := logr.FromContextOrDiscard(ctx)

	pluginId := uuid.NewString()
	if e.current != nil && e.current.Id != nil {
		pluginId = *e.current.Id
	}

	// Order matters: a plugin with a consumer is created for that consumer, on
	// the given route if one is set; a plugin with only a route is created for
	// that route; otherwise it is global.
	//
	// Each variant has its own response type, so the branches unpack the status
	// and the body and share the parsing below. The parsed JSON200 is unused:
	// the consumer variant types it after a schema that truncates the plugin
	// config to two fields.
	var status int
	var body []byte
	var err error
	switch {
	case e.plugin.GetConsumer() != nil:
		log.V(1).Info("upserting plugin for consumer", "consumer", *e.plugin.GetConsumer(), "id", pluginId)
		var response *kong.UpsertPluginForConsumerResponse
		if response, err = e.client.UpsertPluginForConsumerWithResponse(ctx, *e.plugin.GetConsumer(), pluginId, *desired); err == nil {
			status, body = response.StatusCode(), response.Body
		}
	case e.plugin.GetRoute() != nil:
		log.V(1).Info("upserting plugin for route", "route", *e.plugin.GetRoute(), "id", pluginId)
		var response *kong.UpsertPluginForRouteResponse
		if response, err = e.client.UpsertPluginForRouteWithResponse(ctx, *e.plugin.GetRoute(), pluginId, *desired); err == nil {
			status, body = response.StatusCode(), response.Body
		}
	default:
		log.V(1).Info("upserting global plugin", "id", pluginId)
		var response *kong.UpsertPluginResponse
		if response, err = e.client.UpsertPluginWithResponse(ctx, pluginId, *desired); err == nil {
			status, body = response.StatusCode(), response.Body
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to write plugin: %w", HandleClientError(err))
	}

	var written *kong.Plugin
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &written); err != nil {
			return nil, fmt.Errorf("failed to unmarshal plugin response: %w", err)
		}
		if written != nil && written.Id == nil {
			written.Id = &pluginId
		}
	}
	return writeOne("plugin", readResult[kong.Plugin]{status, body, written}, http.StatusOK)
}
