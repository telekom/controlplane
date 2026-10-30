// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/util"
	"go.uber.org/zap"
)

const contentTypeJSON = "application/json"

// StatusError is returned for unexpected Kong admin API responses.
type StatusError struct {
	Context    string
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: unexpected status %d: %s", e.Context, e.StatusCode, strings.TrimSpace(e.Body))
}

type response interface {
	StatusCode() int
}

func checkResponse(res response, body []byte, context string) error {
	if util.Is2xx(res) {
		return nil
	}
	return &StatusError{Context: context, StatusCode: res.StatusCode(), Body: string(body)}
}

// ReadState reads the current gateway state for the entities referenced by desired.
// Missing entities (404) are returned as absent instead of failing.
func ReadState(ctx context.Context, client kong.ClientWithResponsesInterface, desired *snapshot.State) (*snapshot.State, error) {
	state := &snapshot.State{RouteName: desired.RouteName}

	routeID := deref(desired.Route.Id)
	routeRes, err := client.GetRouteWithResponse(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("getting route %s: %w", routeID, err)
	}
	if routeRes.StatusCode() != http.StatusNotFound {
		if err := checkResponse(routeRes, routeRes.Body, "getting route "+routeID); err != nil {
			return nil, err
		}
		state.Route = routeRes.JSON200
	}

	serviceID := deref(desired.Service.Id)
	serviceRes, err := client.GetServiceWithResponse(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("getting service %s: %w", serviceID, err)
	}
	if serviceRes.StatusCode() != http.StatusNotFound {
		if err := checkResponse(serviceRes, serviceRes.Body, "getting service "+serviceID); err != nil {
			return nil, err
		}
		state.Service = serviceRes.JSON200
	}

	if state.Route != nil {
		if state.Plugins, err = listRoutePlugins(ctx, client, routeID); err != nil {
			return nil, err
		}
	}

	// Per convention the upstream is named like the route.
	upstreamRef := deref(desired.Route.Name)
	if desired.Upstream != nil {
		upstreamRef = deref(desired.Upstream.Id)
	}
	if upstreamRef == "" {
		return state, nil
	}
	upstreamRes, err := client.GetUpstreamWithResponse(ctx, upstreamRef)
	if err != nil {
		return nil, fmt.Errorf("getting upstream %s: %w", upstreamRef, err)
	}
	if upstreamRes.StatusCode() == http.StatusNotFound {
		return state, nil
	}
	if err := checkResponse(upstreamRes, upstreamRes.Body, "getting upstream "+upstreamRef); err != nil {
		return nil, err
	}
	state.Upstream = upstreamRes.JSON200
	if state.Targets, err = listTargets(ctx, client, deref(state.Upstream.Id)); err != nil {
		return nil, err
	}
	return state, nil
}

func listRoutePlugins(ctx context.Context, client kong.ClientWithResponsesInterface, routeID string) ([]kong.Plugin, error) {
	var plugins []kong.Plugin
	var offset *string
	for {
		res, err := client.ListPluginsForRouteWithResponse(ctx, routeID, &kong.ListPluginsForRouteParams{Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("listing plugins for route %s: %w", routeID, err)
		}
		if err := checkResponse(res, res.Body, "listing plugins for route "+routeID); err != nil {
			return nil, err
		}
		if res.JSON200 == nil {
			return plugins, nil
		}
		if res.JSON200.Data != nil {
			plugins = append(plugins, *res.JSON200.Data...)
		}
		if res.JSON200.Offset == nil || *res.JSON200.Offset == "" {
			return plugins, nil
		}
		offset = res.JSON200.Offset
	}
}

func listTargets(ctx context.Context, client kong.ClientWithResponsesInterface, upstreamID string) ([]kong.Target, error) {
	var targets []kong.Target
	var offset *string
	for {
		res, err := client.ListTargetsForUpstreamWithResponse(ctx, upstreamID, &kong.ListTargetsForUpstreamParams{Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("listing targets for upstream %s: %w", upstreamID, err)
		}
		if err := checkResponse(res, res.Body, "listing targets for upstream "+upstreamID); err != nil {
			return nil, err
		}
		if res.JSON200 == nil {
			return targets, nil
		}
		if res.JSON200.Data != nil {
			targets = append(targets, *res.JSON200.Data...)
		}
		if res.JSON200.Offset == nil || *res.JSON200.Offset == "" {
			return targets, nil
		}
		offset = res.JSON200.Offset
	}
}

// ExecutionError reports the failing operation and the operations that already succeeded.
type ExecutionError struct {
	Failed    Operation
	Succeeded []Operation
	Err       error
}

func (e *ExecutionError) Error() string {
	done := make([]string, len(e.Succeeded))
	for i, op := range e.Succeeded {
		done[i] = op.String()
	}
	succeeded := "none"
	if len(done) > 0 {
		succeeded = strings.Join(done, "; ")
	}
	return fmt.Sprintf("operation %q failed: %v (already succeeded: %s)", e.Failed.String(), e.Err, succeeded)
}

func (e *ExecutionError) Unwrap() error {
	return e.Err
}

// Execute runs the write operations in order and stops at the first failure.
func Execute(ctx context.Context, client kong.ClientWithResponsesInterface, ops []Operation) ([]Operation, error) {
	var done []Operation
	for _, op := range ops {
		if !op.IsWrite() {
			continue
		}
		if err := executeOne(ctx, client, op); err != nil {
			return done, &ExecutionError{Failed: op, Succeeded: done, Err: err}
		}
		zap.L().Info("executed operation", zap.String("action", string(op.Action)), zap.String("kind", string(op.Kind)),
			zap.String("id", op.ID), zap.String("name", op.Name))
		done = append(done, op)
	}
	return done, nil
}

func executeOne(ctx context.Context, client kong.ClientWithResponsesInterface, op Operation) error {
	if op.Action == ActionDelete {
		return executeDelete(ctx, client, op)
	}
	payload, err := toPayload(op.Entity)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}
	body := bytes.NewReader(data)
	switch op.Kind {
	case KindUpstream:
		res, err := client.UpsertUpstreamWithBodyWithResponse(ctx, op.ID, contentTypeJSON, body)
		if err != nil {
			return err
		}
		return checkResponse(res, res.Body, "upserting upstream")
	case KindTarget:
		res, err := client.UpsertTargetForUpstreamWithBodyWithResponse(ctx, op.ParentID, op.ID, contentTypeJSON, body)
		if err != nil {
			return err
		}
		return checkResponse(res, res.Body, "upserting target")
	case KindService:
		res, err := client.UpsertServiceWithBodyWithResponse(ctx, op.ID, contentTypeJSON, body)
		if err != nil {
			return err
		}
		return checkResponse(res, res.Body, "upserting service")
	case KindRoute:
		res, err := client.UpsertRouteWithBodyWithResponse(ctx, op.ID, contentTypeJSON, body)
		if err != nil {
			return err
		}
		return checkResponse(res, res.Body, "upserting route")
	case KindPlugin:
		res, err := client.UpsertPluginWithBodyWithResponse(ctx, op.ID, contentTypeJSON, body)
		if err != nil {
			return err
		}
		return checkResponse(res, res.Body, "upserting plugin")
	default:
		return fmt.Errorf("unsupported entity kind %q", op.Kind)
	}
}

// executeDelete treats 404 as success so that deletes are idempotent.
func executeDelete(ctx context.Context, client kong.ClientWithResponsesInterface, op Operation) error {
	var res response
	var body []byte
	switch op.Kind {
	case KindPlugin:
		r, err := client.DeletePluginWithResponse(ctx, op.ID)
		if err != nil {
			return err
		}
		res, body = r, r.Body
	case KindTarget:
		r, err := client.DeleteUpstreamTargetWithResponse(ctx, op.ParentID, op.ID)
		if err != nil {
			return err
		}
		res, body = r, r.Body
	case KindUpstream:
		r, err := client.DeleteUpstreamWithResponse(ctx, op.ID)
		if err != nil {
			return err
		}
		res, body = r, r.Body
	default:
		return fmt.Errorf("deleting %q is not supported", op.Kind)
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil
	}
	return checkResponse(res, body, "deleting "+string(op.Kind))
}
