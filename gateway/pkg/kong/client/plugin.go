// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

// Kinds of the Kong entities a plugin can be bound to.
const (
	parentConsumer = "consumer"
	parentRoute    = "route"
)

func (c *kongClient) CreateOrReplacePlugin(ctx context.Context, plugin CustomPlugin) (*kong.Plugin, error) {
	pluginName := plugin.GetName()
	pluginEnabled := true

	config, err := normalizeConfig(plugin.GetConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to normalize desired plugin config: %w", err)
	}

	tags := pluginTags(ctx, plugin)
	if prefix := pluginNamePrefix(plugin); prefix != "" {
		tags = append(tags, BuildTag("realm", prefix))
	}
	desired := kong.UpsertPluginJSONRequestBody{
		Enabled:   &pluginEnabled,
		Name:      &pluginName,
		Protocols: convertStringSet[kong.CreatePluginForConsumerRequestProtocols](&[]string{"http"}),
		Tags:      normalizeSet(&tags),
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
		if routeID := boundRouteID(plugin); routeID != "" {
			desired.Route = &map[string]any{"id": routeID}
		} else {
			desired.Route = &map[string]any{"name": plugin.GetRoute()}
		}
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
		if kongPlugin.Id == nil {
			return fmt.Errorf("plugin response ID is missing")
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

// loadGlobalPlugin looks up a plugin bound to neither a route nor a consumer
// by its stored ID and falls back to its ownership tags.
func loadGlobalPlugin(ctx context.Context, api KongAdminApi, plugin CustomPlugin) (*kong.Plugin, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("plugin", plugin.GetName())

	if pluginId := plugin.GetId(); pluginId != "" {
		log.V(1).Info("loading plugin by id", "id", pluginId)
		current, err := fetchPlugin(ctx, api, pluginId)
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
		if current.Id == nil || *current.Id == "" {
			return nil, fmt.Errorf("plugin response ID is missing")
		}
		log.V(1).Info("found plugin", "id", *current.Id)
	}
	return current, nil
}

// boundRouteID returns the Kong route ID a plugin is bound to, or "" if it is bound by name.
func boundRouteID(binder any) string {
	if b, ok := binder.(RouteIdBinder); ok {
		return b.GetRouteId()
	}
	return ""
}

// routeRef returns the Kong route ID of a route-bound plugin, or its route name.
func routeRef(plugin CustomPlugin) string {
	if routeID := boundRouteID(plugin); routeID != "" {
		return routeID
	}
	return *plugin.GetRoute()
}

// pluginNamePrefix returns the name prefix of a route-bound plugin, or "".
func pluginNamePrefix(plugin CustomPlugin) string {
	realm, ok := plugin.(RealmPlugin)
	if !ok || plugin.GetRoute() == nil {
		return ""
	}
	return realm.GetNamePrefix()
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
		tags = append(tags, BuildTag(parentRoute, *plugin.GetRoute()))
	}
	if plugin.GetConsumer() != nil {
		tags = append(tags, BuildTag(parentConsumer, *plugin.GetConsumer()))
	} else {
		tags = append(tags, BuildTag(parentConsumer, "none"))
	}
	return tags
}

func getPluginMatchingTags(ctx context.Context, api KongAdminApi, tags []string) (*kong.Plugin, error) {
	plugins, err := listPlugins(ctx, api, tags)
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

// resolveParentID returns the Kong ID of the route or consumer a plugin is
// bound to. A missing parent is an error.
func resolveParentID(ctx context.Context, api KongAdminApi, kind, name string) (string, error) {
	id, found, err := lookupParentID(ctx, api, kind, name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s %q does not exist in Kong", kind, name)
	}
	return id, nil
}

// lookupParentID returns the Kong ID of a route or consumer and whether Kong
// knows it. A found parent without an ID is an error.
func lookupParentID(ctx context.Context, api KongAdminApi, kind, name string) (string, bool, error) {
	var id *string
	switch kind {
	case parentConsumer:
		response, err := api.GetConsumerWithResponse(ctx, name)
		if err != nil {
			return "", false, fmt.Errorf("failed to get consumer %q: %w", name, HandleClientError(err))
		}
		consumer, found, err := readOne("consumer", readResult[kong.Consumer]{response.StatusCode(), response.Body, response.JSON200})
		if err != nil || !found {
			return "", false, err
		}
		id = consumer.Id
	case parentRoute:
		response, err := api.GetRouteWithResponse(ctx, name)
		if err != nil {
			return "", false, fmt.Errorf("failed to get route %q: %w", name, HandleClientError(err))
		}
		route, found, err := readOne("route", readResult[kong.Route]{response.StatusCode(), response.Body, response.JSON200})
		if err != nil || !found {
			return "", false, err
		}
		id = route.Id
	default:
		return "", false, fmt.Errorf("unknown plugin parent kind %q", kind)
	}
	if id == nil || *id == "" {
		return "", false, fmt.Errorf("%s %q response ID is missing", kind, name)
	}
	return *id, true, nil
}

type pluginEntity struct {
	client     KongAdminApi
	plugin     CustomPlugin
	current    *kong.Plugin
	consumerID *string
	routeID    *string
}

var (
	_ entity[kong.UpsertPluginJSONRequestBody, kong.Plugin] = &pluginEntity{}
	_ comparer[kong.UpsertPluginJSONRequestBody]            = &pluginEntity{}
)

func (e *pluginEntity) Name() string { return "plugin" }

func (e *pluginEntity) bound() bool {
	return e.plugin.GetRoute() != nil || e.plugin.GetConsumer() != nil
}

// Get resolves the parents of a bound plugin and then looks the plugin up by
// its stored ID, its ownership tags and, for a route-bound plugin, its slot on
// the route. A global plugin is looked up by its stored ID and its ownership tags.
func (e *pluginEntity) Get(ctx context.Context) (*kong.Plugin, bool, error) {
	e.current = nil
	e.consumerID = nil
	e.routeID = nil

	if !e.bound() {
		current, err := loadGlobalPlugin(ctx, e.client, e.plugin)
		if err != nil {
			return nil, false, err
		}
		e.current = current
		return current, current != nil, nil
	}

	identity := &boundPluginIdentity{
		name: e.plugin.GetName(),
		tags: pluginTags(ctx, e.plugin),
	}
	if e.plugin.GetConsumer() != nil {
		consumerID, err := resolveParentID(ctx, e.client, parentConsumer, *e.plugin.GetConsumer())
		if err != nil {
			return nil, false, err
		}
		e.consumerID = &consumerID
		identity.consumerID = consumerID
	}
	if e.plugin.GetRoute() != nil {
		routeID, err := resolveParentID(ctx, e.client, parentRoute, routeRef(e.plugin))
		if err != nil {
			return nil, false, err
		}
		e.routeID = &routeID
		identity.routeID = routeID
	}

	current, err := discoverBoundPlugin(ctx, e.client, e.plugin.GetId(), identity)
	if err != nil {
		return nil, false, err
	}
	e.current = current
	return current, current != nil, nil
}

// effective returns the desired body for the plugin as it will be written. An
// existing bound plugin keeps all its tags, its protocols and its instance
// name. The config is the desired config and replaces the current one.
func (e *pluginEntity) effective(desired kong.UpsertPluginJSONRequestBody) kong.UpsertPluginJSONRequestBody {
	if e.current == nil || !e.bound() {
		return desired
	}
	tags := append(slices.Clone(valueOrZero(e.current.Tags)), valueOrZero(desired.Tags)...)
	desired.Tags = normalizeSet(&tags)
	desired.Protocols = convertStringSet[kong.CreatePluginForConsumerRequestProtocols](e.current.Protocols)
	desired.InstanceName = e.current.InstanceName
	return desired
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
	if current.Consumer != nil && current.Consumer.Id != nil {
		projected.Consumer = current.Consumer.Id
	}
	if current.Route != nil && current.Route.Id != nil {
		projected.Route = &map[string]any{"id": *current.Route.Id}
	}
	if current.Service != nil && current.Service.Id != nil {
		projected.Service = current.Service.Id
	}
	if config != nil {
		projected.Config = &config
	}
	return projected, nil
}

// Equal compares the request against the current definition while normalizing
// the entity references that Kong reports as IDs. If Kong drifted the plugin to
// a different route or consumer out-of-band, matching on the name would be a
// false positive because the ownership tags alone do not make the plugin safe to
// reuse. An existing bound plugin is compared against its effective desired
// body, so missing ownership tags still count as a change.
func (e *pluginEntity) Equal(desired, current kong.UpsertPluginJSONRequestBody) bool {
	desired = e.effective(desired)
	desired.Service = nil
	current.Service = nil
	if e.consumerID != nil {
		desired.Consumer = e.consumerID
	}
	if e.routeID != nil {
		desired.Route = &map[string]any{"id": *e.routeID}
	}

	desiredConfig := valueOrZero(desired.Config)
	narrowed := narrowToDesired(desiredConfig, valueOrZero(current.Config))
	desired.Config, current.Config = &desiredConfig, &narrowed

	return reflect.DeepEqual(desired, current)
}

func (e *pluginEntity) Write(ctx context.Context, desired *kong.UpsertPluginJSONRequestBody) (*kong.Plugin, error) {
	log := logr.FromContextOrDiscard(ctx)

	pluginId := uuid.NewString()
	if e.current != nil {
		if e.current.Id == nil || *e.current.Id == "" {
			return nil, fmt.Errorf("existing plugin ID is missing")
		}
		pluginId = *e.current.Id
	}
	effective := e.effective(*desired)
	desired = &effective

	// Order matters: a plugin with a consumer is created for that consumer, on
	// the given route if one is set; a plugin with only a route is created for
	// that route; otherwise it is global.
	//
	// Each variant has its own response type, so the branches unpack the status
	// and the body and share the parsing below into the canonical kong.Plugin.
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
		log.V(1).Info("upserting plugin for route", "route", routeRef(e.plugin), "id", pluginId)
		var response *kong.UpsertPluginForRouteResponse
		if response, err = e.client.UpsertPluginForRouteWithResponse(ctx, routeRef(e.plugin), pluginId, *desired); err == nil {
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
		if written != nil && *written.Id != pluginId {
			return nil, fmt.Errorf("plugin write for id %s returned id %s", pluginId, *written.Id)
		}
	}
	return writeOne("plugin", readResult[kong.Plugin]{status, body, written}, http.StatusOK)
}
