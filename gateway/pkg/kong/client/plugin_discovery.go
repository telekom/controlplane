// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-logr/logr"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

// LegacyPluginTag is the realm tag the legacy operator stamped on its plugins.
// A bound plugin that is found neither by its stored ID nor by its ownership
// tags is looked up by legacyPluginTags and adopted under its existing ID.
const LegacyPluginTag = "ei__telekom__de--realm---default"

const pluginListPageSize = 1000

// ownershipNamespaces are the tag namespaces that carry plugin ownership.
var ownershipNamespaces = []string{"env", "plugin", parentRoute, parentConsumer}

// listPlugins reads every page of the plugins matching all tags.
func listPlugins(ctx context.Context, api KongAdminApi, tags []string) ([]kong.Plugin, error) {
	var plugins []kong.Plugin
	var offset *string
	seen := map[string]bool{}
	for {
		size := pluginListPageSize
		// ListPluginsForRouteWithResponse does not work correctly with tags
		response, err := api.ListPluginWithResponse(ctx, &kong.ListPluginParams{Size: &size, Offset: offset, Tags: encodeTags(tags)})
		if err != nil {
			return nil, HandleClientError(err)
		}
		if err := CheckStatusCode(response, http.StatusOK); err != nil {
			return nil, fmt.Errorf("failed to list plugins (%d): %s: %w", response.StatusCode(), summarizeBody(response.Body), err)
		}
		// ListPluginWithResponse does not return an array of plugins
		var page struct {
			Data   []kong.Plugin `json:"data"`
			Offset *string       `json:"offset"`
		}
		if err := json.Unmarshal(response.Body, &page); err != nil {
			return nil, fmt.Errorf("failed to decode plugin list: %w", err)
		}
		if page.Data == nil {
			return nil, errors.New("failed to list plugins: response data is missing")
		}
		plugins = append(plugins, page.Data...)
		if page.Offset == nil || *page.Offset == "" {
			return plugins, nil
		}
		if seen[*page.Offset] {
			return nil, errors.New("failed to list plugins: offset repeats")
		}
		seen[*page.Offset] = true
		offset = page.Offset
	}
}

// checkPlugin rejects a plugin without ID or with an association without ID.
func checkPlugin(plugin *kong.Plugin) error {
	id := valueOrZero(plugin.Id)
	if id == "" {
		return errors.New("plugin record has no id")
	}
	if (plugin.Route != nil && valueOrZero(plugin.Route.Id) == "") ||
		(plugin.Consumer != nil && valueOrZero(plugin.Consumer.Id) == "") ||
		(plugin.Service != nil && valueOrZero(plugin.Service.Id) == "") {
		return fmt.Errorf("plugin %s has an association without id", id)
	}
	return nil
}

// fetchPlugin reads a plugin by ID. Only a 404 reports it as absent.
func fetchPlugin(ctx context.Context, api KongAdminApi, pluginId string) (*kong.Plugin, error) {
	response, err := api.GetPluginWithResponse(ctx, pluginId)
	if err != nil {
		return nil, HandleClientError(err)
	}
	plugin, found, err := readOne("plugin", readResult[kong.Plugin]{response.StatusCode(), response.Body, response.JSON200})
	if err != nil || !found {
		return nil, err
	}
	if err := checkPlugin(plugin); err != nil {
		return nil, err
	}
	if *plugin.Id != pluginId {
		return nil, fmt.Errorf("plugin lookup for id %s returned unrelated id %s", pluginId, *plugin.Id)
	}
	return plugin, nil
}

// ownershipTags returns the sorted, deduplicated ownership tags of a plugin.
func ownershipTags(tags []string) []string {
	var owned []string
	for _, tag := range tags {
		namespace, _, _ := strings.Cut(tag, "--")
		if slices.Contains(ownershipNamespaces, namespace) {
			owned = append(owned, tag)
		}
	}
	slices.Sort(owned)
	return slices.Compact(owned)
}

func pluginRouteID(plugin *kong.Plugin) string {
	if plugin.Route == nil {
		return ""
	}
	return valueOrZero(plugin.Route.Id)
}

func pluginConsumerID(plugin *kong.Plugin) string {
	if plugin.Consumer == nil {
		return ""
	}
	return valueOrZero(plugin.Consumer.Id)
}

// boundPluginIdentity is the slot of a route- or consumer-bound plugin and the
// ownership tags that prove a Kong plugin belongs to it.
type boundPluginIdentity struct {
	name       string
	tags       []string
	routeID    string
	consumerID string
}

// goOwned reports whether the plugin carries exactly the desired ownership
// tags. Drifted route and consumer associations are repaired in place.
func (i *boundPluginIdentity) goOwned(plugin *kong.Plugin) bool {
	return valueOrZero(plugin.Name) == i.name && plugin.Service == nil &&
		slices.Equal(ownershipTags(valueOrZero(plugin.Tags)), ownershipTags(i.tags))
}

// inSlot reports whether the plugin has the desired name and is bound to
// exactly the desired route and consumer.
func (i *boundPluginIdentity) inSlot(plugin *kong.Plugin) bool {
	return valueOrZero(plugin.Name) == i.name && plugin.Service == nil &&
		pluginRouteID(plugin) == i.routeID && pluginConsumerID(plugin) == i.consumerID
}

// legacyOwned reports whether the plugin is a legacy plugin of exactly this
// slot whose ownership tags do not conflict with the desired ones.
func (i *boundPluginIdentity) legacyOwned(plugin *kong.Plugin) bool {
	if !slices.Contains(valueOrZero(plugin.Tags), LegacyPluginTag) || !i.inSlot(plugin) {
		return false
	}
	for _, tag := range ownershipTags(valueOrZero(plugin.Tags)) {
		if !slices.Contains(i.tags, tag) {
			return false
		}
	}
	return true
}

// discoverBoundPlugin looks up a bound plugin by its stored ID, then by its
// ownership tags and finally by its legacy tags. It returns nil if none
// matches.
func discoverBoundPlugin(ctx context.Context, api KongAdminApi, plugin CustomPlugin, identity *boundPluginIdentity) (*kong.Plugin, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("plugin", plugin.GetName())

	if pluginId := plugin.GetId(); pluginId != "" {
		log.V(1).Info("loading plugin by id", "id", pluginId)
		current, err := fetchPlugin(ctx, api, pluginId)
		if err != nil {
			return nil, err
		}
		if current != nil {
			if identity.goOwned(current) || identity.legacyOwned(current) {
				return current, nil
			}
			return nil, fmt.Errorf("plugin %s stored for %s is not owned by it", pluginId, plugin.GetName())
		}
		log.V(1).Info("plugin not found", "id", pluginId)
	}

	log.V(1).Info("loading plugin by tags", "tags", identity.tags)
	current, err := findOne(ctx, api, identity.tags, identity.name, func(candidate *kong.Plugin) (bool, error) {
		if !identity.goOwned(candidate) {
			return false, fmt.Errorf("plugin %s matched the ownership tags of %s but carries inconsistent ownership", *candidate.Id, identity.name)
		}
		return true, nil
	})
	if err != nil || current != nil {
		return current, err
	}

	legacyTags := legacyPluginTags(plugin)
	log.V(1).Info("loading plugin by legacy tags", "tags", legacyTags)
	return findOne(ctx, api, legacyTags, identity.name, func(candidate *kong.Plugin) (bool, error) {
		if !identity.inSlot(candidate) {
			return false, nil
		}
		if !identity.legacyOwned(candidate) {
			return false, fmt.Errorf("legacy plugin %s for %s carries inconsistent ownership", *candidate.Id, identity.name)
		}
		return true, nil
	})
}

// legacyPluginTags returns the tags the legacy operator stamped on a bound
// plugin: the realm tag, the plugin name and the route name and consumer
// username it is bound to.
func legacyPluginTags(plugin CustomPlugin) []string {
	tags := []string{LegacyPluginTag, plugin.GetName()}
	if plugin.GetRoute() != nil {
		tags = append(tags, *plugin.GetRoute())
	}
	if plugin.GetConsumer() != nil {
		tags = append(tags, *plugin.GetConsumer())
	}
	return tags
}

// findOne lists the plugins matching the tags and returns the single plugin
// with the given name that match accepts. Plugins with another name are
// ignored; more than one accepted plugin is an error.
func findOne(ctx context.Context, api KongAdminApi, tags []string, name string, match func(*kong.Plugin) (bool, error)) (*kong.Plugin, error) {
	plugins, err := listPlugins(ctx, api, tags)
	if err != nil {
		return nil, err
	}
	var found *kong.Plugin
	for i := range plugins {
		candidate := &plugins[i]
		if valueOrZero(candidate.Name) != name {
			continue
		}
		if err := checkPlugin(candidate); err != nil {
			return nil, err
		}
		ok, err := match(candidate)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("found multiple %s plugins with tags: %s", name, *encodeTags(tags))
		}
		found = candidate
	}
	return found, nil
}
