// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

// CleanupPlugins deletes the plugins of one route or consumer that are not
// desired anymore. Only plugins whose Go ownership tags and actual bindings
// prove they belong to the scope are deleted; nothing is deleted unless every
// candidate could be classified.
func (c *kongClient) CleanupPlugins(ctx context.Context, route CustomRoute, consumer CustomConsumer, plugins []CustomPlugin) error {
	log := logr.FromContextOrDiscard(ctx)

	if route == nil && consumer == nil {
		return errors.New("either route or consumer must be provided for cleanup")
	}
	if route != nil && consumer != nil {
		return errors.New("only one of route or consumer may be provided for cleanup")
	}

	desiredIds := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		if plugin.GetId() == "" {
			return fmt.Errorf("plugin %s has no id; refusing to clean up", plugin.GetName())
		}
		desiredIds = append(desiredIds, plugin.GetId())
	}

	scope := cleanupScope{env: contextutil.EnvFromContextOrDie(ctx), kind: parentRoute}
	parentRef := ""
	if route != nil {
		scope.name = route.GetName()
		parentRef = scope.name
		if routeID := boundRouteID(route); routeID != "" {
			parentRef = routeID
		}
	} else {
		scope.kind, scope.name = parentConsumer, consumer.GetConsumerName()
		parentRef = scope.name
	}
	parentID, err := resolveParentID(ctx, c.client, scope.kind, parentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve cleanup scope: %w", err)
	}
	scope.parentID = parentID

	kongPlugins, err := listPlugins(ctx, c.client, []string{BuildTag("env", scope.env), BuildTag(scope.kind, scope.name)})
	if err != nil {
		return fmt.Errorf("failed to list plugins: %w", err)
	}

	var obsolete []string
	for i := range kongPlugins {
		plugin := &kongPlugins[i]
		if slices.Contains(desiredIds, valueOrZero(plugin.Id)) {
			continue
		}
		if err := checkPlugin(plugin); err != nil {
			return fmt.Errorf("failed to classify plugins: %w", err)
		}
		if slices.Contains(obsolete, *plugin.Id) {
			return fmt.Errorf("failed to classify plugins: id %s is listed more than once", *plugin.Id)
		}
		owned, err := scope.owns(ctx, c.client, plugin)
		if err != nil {
			return fmt.Errorf("failed to classify plugin %s: %w", *plugin.Id, err)
		}
		if !owned {
			log.V(1).Info("skipping plugin without provable ownership", "id", *plugin.Id, "name", valueOrZero(plugin.Name))
			continue
		}
		obsolete = append(obsolete, *plugin.Id)
	}
	slices.Sort(obsolete)

	log.Info("cleaning up plugins", "found", len(kongPlugins), "expected", len(desiredIds), "obsolete", len(obsolete))

	for _, id := range obsolete {
		log.V(1).Info("deleting plugin", "id", id)
		response, err := c.client.DeletePluginWithResponse(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to delete plugin %s: %w", id, HandleClientError(err))
		}
		if err := CheckStatusCode(response, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
			return fmt.Errorf("failed to delete plugin %s (%d): %s: %w", id, response.StatusCode(), summarizeBody(response.Body), err)
		}
	}
	return nil
}

// cleanupScope is the route or consumer whose obsolete plugins are removed.
type cleanupScope struct {
	env, kind, name, parentID string
}

// owns reports whether the plugin provably is a plugin of the scope. A consumer
// scope never owns a plugin that is also bound to a route; such a plugin
// belongs to the route.
func (s *cleanupScope) owns(ctx context.Context, api KongAdminApi, plugin *kong.Plugin) (bool, error) {
	tags := valueOrZero(plugin.Tags)
	if plugin.Service != nil || !singleTag(tags, "env", s.env) || !singleTag(tags, "plugin", valueOrZero(plugin.Name)) {
		return false, nil
	}
	if s.kind == parentConsumer {
		return singleTag(tags, parentConsumer, s.name) && pluginConsumerID(plugin) == s.parentID &&
			plugin.Route == nil && len(tagValues(tags, parentRoute)) == 0, nil
	}

	if !singleTag(tags, parentRoute, s.name) || pluginRouteID(plugin) != s.parentID {
		return false, nil
	}
	consumers := tagValues(tags, parentConsumer)
	if len(consumers) != 1 {
		return false, nil
	}
	if consumers[0] == "none" || plugin.Consumer == nil {
		return consumers[0] == "none" && plugin.Consumer == nil, nil
	}
	consumerID, found, err := lookupParentID(ctx, api, parentConsumer, consumers[0])
	if err != nil {
		return false, err
	}
	return found && consumerID == pluginConsumerID(plugin), nil
}

// tagValues returns the distinct values of the tags in the namespace.
func tagValues(tags []string, namespace string) []string {
	var values []string
	for _, tag := range ownershipTags(tags) {
		if value, found := strings.CutPrefix(tag, namespace+"--"); found {
			values = append(values, value)
		}
	}
	return values
}

func singleTag(tags []string, namespace, value string) bool {
	values := tagValues(tags, namespace)
	return len(values) == 1 && values[0] == value
}
