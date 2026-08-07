// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

// CreateOrReplaceUpstream normalizes the given bodies in place before comparing
// them, so the caller must not reuse them afterwards.
func (c *kongClient) CreateOrReplaceUpstream(ctx context.Context, route CustomRoute, upstream *kong.CreateUpstreamJSONRequestBody, target *kong.CreateTargetForUpstreamJSONRequestBody) error {
	if upstream == nil {
		return fmt.Errorf("upstream request body is missing")
	}
	if target == nil {
		return fmt.Errorf("target request body is missing")
	}

	upstream.Tags = normalizeSet(upstream.Tags)
	normalizeDesiredHealthchecks(upstream.Healthchecks)

	kongUpstream, _, err := reconcile(ctx, upstreamEntity{client: c.client, name: upstream.Name}, *upstream)
	if err != nil {
		return err
	}
	if kongUpstream.Id == nil {
		return fmt.Errorf("upstream response ID is missing")
	}
	route.SetUpstreamId(*kongUpstream.Id)

	target.Tags = normalizeSet(target.Tags)
	kongTarget, _, err := reconcile(ctx, targetEntity{
		client:       c.client,
		upstreamName: upstream.Name,
		address:      target.Target,
		tags:         target.Tags,
	}, *target)
	if err != nil {
		return err
	}
	if kongTarget.Id == nil {
		return fmt.Errorf("target response ID is missing")
	}
	route.SetTargetsId(*kongTarget.Id)

	return nil
}

func (c *kongClient) DeleteUpstream(ctx context.Context, route CustomRoute) error {
	upstreamResponse, err := c.client.DeleteUpstreamWithResponse(ctx, route.GetName())
	if err != nil {
		return fmt.Errorf("failed to delete upstream: %w", HandleClientError(err))
	}
	if err := CheckStatusCode(upstreamResponse, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
		return fmt.Errorf("failed to delete upstream (%d): %s: %w", upstreamResponse.StatusCode(), summarizeBody(upstreamResponse.Body), err)
	}

	if route.GetTargetsId() != "" {
		// targets don't have names, so we use the ID directly
		targetsResponse, err := c.client.DeleteUpstreamTargetWithResponse(ctx, route.GetName(), route.GetTargetsId())
		if err != nil {
			return fmt.Errorf("failed to delete upstream targets: %w", HandleClientError(err))
		}
		if err := CheckStatusCode(targetsResponse, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
			return fmt.Errorf("failed to delete upstream targets (%d): %s: %w", targetsResponse.StatusCode(), summarizeBody(targetsResponse.Body), err)
		}
	}
	return nil
}

type upstreamEntity struct {
	client KongAdminApi
	name   string
}

var _ entity[kong.CreateUpstreamJSONRequestBody, kong.Upstream] = upstreamEntity{}

func (e upstreamEntity) Name() string { return "upstream" }

func (e upstreamEntity) Get(ctx context.Context) (*kong.Upstream, bool, error) {
	response, err := e.client.GetUpstreamWithResponse(ctx, e.name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get upstream: %w", HandleClientError(err))
	}
	return readOne("upstream", readResult[kong.Upstream]{response.StatusCode(), response.Body, response.JSON200})
}

func (e upstreamEntity) Project(current *kong.Upstream) (kong.CreateUpstreamJSONRequestBody, error) {
	projected := kong.CreateUpstreamJSONRequestBody{
		Name:         valueOrZero(current.Name),
		Healthchecks: projectHealthchecks(current.Healthchecks),
		Tags:         normalizeSet(current.Tags),
	}
	if current.Algorithm != nil {
		algorithm := kong.CreateUpstreamRequestAlgorithm(*current.Algorithm)
		projected.Algorithm = &algorithm
	}
	return projected, nil
}

func (e upstreamEntity) Write(ctx context.Context, desired *kong.CreateUpstreamJSONRequestBody) (*kong.Upstream, error) {
	response, err := e.client.UpsertUpstreamWithResponse(ctx, e.name, *desired)
	if err != nil {
		return nil, fmt.Errorf("failed to write upstream: %w", HandleClientError(err))
	}
	return writeOne("upstream", readResult[kong.Upstream]{response.StatusCode(), response.Body, response.JSON200}, http.StatusOK)
}

// projectHealthchecks copies only the health-check fields the circuit-breaker
// feature sets. Kong fills in defaults for every field the controller leaves
// unset, so a projection that copied the whole response - for example by
// decoding it into the request type - would never equal the desired body and
// every reconciliation would write. A group whose managed fields are all unset
// is dropped for the same reason.
func projectHealthchecks(current *kong.UpstreamHealthchecks) *kong.CreateUpstreamRequestHealthchecks {
	if current == nil {
		return nil
	}

	projected := kong.CreateUpstreamRequestHealthchecks{}

	if current.Active != nil {
		active := kong.CreateUpstreamRequestHealthchecksActive{
			Type: convertString[kong.CreateUpstreamRequestHealthchecksActiveType](current.Active.Type),
		}
		if current.Active.Healthy != nil {
			active.Healthy = nilIfZero(kong.CreateUpstreamRequestHealthchecksActiveHealthy{
				HttpStatuses: normalizeSet(current.Active.Healthy.HttpStatuses),
			})
		}
		if current.Active.Unhealthy != nil {
			active.Unhealthy = nilIfZero(kong.CreateUpstreamRequestHealthchecksActiveUnhealthy{
				HttpStatuses: normalizeSet(current.Active.Unhealthy.HttpStatuses),
			})
		}
		projected.Active = nilIfZero(active)
	}

	if current.Passive != nil {
		passive := kong.CreateUpstreamRequestHealthchecksPassive{
			Type: convertString[kong.CreateUpstreamRequestHealthchecksPassiveType](current.Passive.Type),
		}
		if current.Passive.Healthy != nil {
			passive.Healthy = nilIfZero(kong.CreateUpstreamRequestHealthchecksPassiveHealthy{
				HttpStatuses: convertIntSet[kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses](current.Passive.Healthy.HttpStatuses),
				Successes:    current.Passive.Healthy.Successes,
			})
		}
		if current.Passive.Unhealthy != nil {
			passive.Unhealthy = nilIfZero(kong.CreateUpstreamRequestHealthchecksPassiveUnhealthy{
				HttpFailures: current.Passive.Unhealthy.HttpFailures,
				HttpStatuses: convertIntSet[kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses](current.Passive.Unhealthy.HttpStatuses),
				TcpFailures:  current.Passive.Unhealthy.TcpFailures,
				Timeouts:     current.Passive.Unhealthy.Timeouts,
			})
		}
		projected.Passive = nilIfZero(passive)
	}

	return nilIfZero(projected)
}

// normalizeDesiredHealthchecks sorts the status sets of a desired body so it can
// be compared with a projection of Kong's response.
func normalizeDesiredHealthchecks(healthchecks *kong.CreateUpstreamRequestHealthchecks) {
	if healthchecks == nil {
		return
	}
	if healthchecks.Active != nil {
		if healthchecks.Active.Healthy != nil {
			healthchecks.Active.Healthy.HttpStatuses = normalizeSet(healthchecks.Active.Healthy.HttpStatuses)
		}
		if healthchecks.Active.Unhealthy != nil {
			healthchecks.Active.Unhealthy.HttpStatuses = normalizeSet(healthchecks.Active.Unhealthy.HttpStatuses)
		}
	}
	if healthchecks.Passive != nil {
		if healthchecks.Passive.Healthy != nil {
			healthchecks.Passive.Healthy.HttpStatuses = normalizeSet(healthchecks.Passive.Healthy.HttpStatuses)
		}
		if healthchecks.Passive.Unhealthy != nil {
			healthchecks.Passive.Unhealthy.HttpStatuses = normalizeSet(healthchecks.Passive.Unhealthy.HttpStatuses)
		}
	}
}

// nilIfZero drops a projected group that carries no managed value. The health
// check types hold only pointers, so the zero value means "nothing projected".
func nilIfZero[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}

func convertString[Out ~string](value *string) *Out {
	if value == nil {
		return nil
	}
	converted := Out(*value)
	return &converted
}

// --- target ----------------------------------------------------------------

// targetEntity reconciles the target of an upstream. Kong targets are
// append-only: posting a target adds a new entry and the most recent one for an
// address decides the effective weight, so the current state is found by
// listing rather than by a lookup on a stable name.
type targetEntity struct {
	client       KongAdminApi
	upstreamName string
	address      *string
	tags         *[]string
}

var _ entity[kong.CreateTargetForUpstreamJSONRequestBody, kong.Target] = targetEntity{}

func (e targetEntity) Name() string { return "target" }

func (e targetEntity) Get(ctx context.Context) (*kong.Target, bool, error) {
	size := 1000
	params := &kong.ListTargetsForUpstreamParams{Size: &size}
	if e.tags != nil && len(*e.tags) > 0 {
		tags := strings.Join(*e.tags, ",")
		params.Tags = &tags
	}

	var effective *kong.Target
	for {
		response, err := e.client.ListTargetsForUpstreamWithResponse(ctx, e.upstreamName, params)
		if err != nil {
			return nil, false, fmt.Errorf("failed to list upstream targets: %w", HandleClientError(err))
		}
		if err := CheckStatusCode(response, http.StatusOK); err != nil {
			return nil, false, fmt.Errorf("failed to list upstream targets (%d): %s: %w", response.StatusCode(), summarizeBody(response.Body), err)
		}
		if response.JSON200 == nil {
			return nil, false, fmt.Errorf("target list response body is missing")
		}
		if response.JSON200.Data == nil {
			return nil, false, fmt.Errorf("target list response data is missing")
		}

		for i := range *response.JSON200.Data {
			candidate := &(*response.JSON200.Data)[i]
			if equalString(candidate.Target, e.address) && isLaterTarget(candidate, effective) {
				effective = candidate
			}
		}

		if response.JSON200.Offset == nil || *response.JSON200.Offset == "" {
			return effective, effective != nil, nil
		}
		if params.Offset != nil && *response.JSON200.Offset == *params.Offset {
			return nil, false, fmt.Errorf("target list pagination offset did not advance")
		}
		params.Offset = response.JSON200.Offset
	}
}

func (e targetEntity) Project(current *kong.Target) (kong.CreateTargetForUpstreamJSONRequestBody, error) {
	return kong.CreateTargetForUpstreamJSONRequestBody{
		Target: current.Target,
		Weight: current.Weight,
		Tags:   normalizeSet(current.Tags),
	}, nil
}

func (e targetEntity) Write(ctx context.Context, desired *kong.CreateTargetForUpstreamJSONRequestBody) (*kong.Target, error) {
	response, err := e.client.CreateTargetForUpstreamWithResponse(ctx, e.upstreamName, *desired)
	if err != nil {
		return nil, fmt.Errorf("failed to write target: %w", HandleClientError(err))
	}
	return writeOne("target", readResult[kong.Target]{response.StatusCode(), response.Body, response.JSON200},
		http.StatusOK, http.StatusCreated)
}

// isLaterTarget reports whether candidate supersedes effective. Targets created
// within the same timestamp tick are ordered by id, so the choice does not
// depend on the order Kong happens to list them in.
func isLaterTarget(candidate, effective *kong.Target) bool {
	if effective == nil || effective.CreatedAt == nil {
		return true
	}
	if candidate.CreatedAt == nil {
		return false
	}
	if *candidate.CreatedAt != *effective.CreatedAt {
		return *candidate.CreatedAt > *effective.CreatedAt
	}
	return valueOrZero(candidate.Id) > valueOrZero(effective.Id)
}

func equalString(left, right *string) bool {
	return left != nil && right != nil && *left == *right
}

// --- plugin ----------------------------------------------------------------
