// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/gateway/pkg/kong/migration"
)

// entity describes how a single Kong entity is read, projected back into its
// request representation and written. Req is the generated request-body type,
// Resp the generated response type. The generated request types are large, so
// the desired body is passed to Write by pointer.
type entity[Req, Resp any] interface {
	Name() string
	Get(ctx context.Context) (current *Resp, found bool, err error)
	Project(current *Resp) (Req, error)
	Write(ctx context.Context, desired *Req) (*Resp, error)
}

// comparer is an optional capability of an entity. Entities whose request type
// carries fields the controller does not manage implement it to exclude those
// fields, so that a Kong-side default can never be mistaken for a change. It returns
// the desired and current bodies as they are compared.
type comparer[Req any] interface {
	Comparable(desired, current Req) (Req, Req)
}

// reconcile writes desired to Kong only if it differs from what Kong already
// holds. It returns the current or written entity and whether a write happened.
// A non-nil guard checks the write against the current state first (Migration Mode).
func reconcile[Req, Resp any](ctx context.Context, guard *migration.Guard, e entity[Req, Resp], desired Req) (result *Resp, written bool, err error) {
	outcome := "error"
	defer func() { kongReconcileTotal.WithLabelValues(e.Name(), outcome).Inc() }()

	current, found, err := e.Get(ctx)
	if err != nil {
		return nil, false, err
	}

	compared := desired
	var projected Req
	if found {
		var projectErr error
		projected, projectErr = e.Project(current)
		if projectErr != nil {
			return nil, false, projectErr
		}
		compared, projected = comparedBodies(e, desired, projected)
		if reflect.DeepEqual(compared, projected) {
			outcome = "unchanged"
			return current, false, nil
		}
	}
	name := migration.NameOf(desired)
	write, guardErr := guard.CheckWrite(ctx, e.Name(), name, compared, projected, found)
	if guardErr != nil {
		outcome = "blocked"
		return nil, false, guardErr
	}
	if !write {
		outcome = "planned"
		if found {
			return current, false, nil
		}
		planned, plannedErr := plannedEntity[Resp](ctx, e.Name(), name, desired)
		return planned, false, plannedErr
	}

	logr.FromContextOrDiscard(ctx).V(1).Info("writing kong entity", "entity", e.Name(), "exists", found)
	result, err = e.Write(ctx, &desired)
	if err != nil {
		return nil, false, err
	}
	outcome = "written"
	return result, true, nil
}

// comparedBodies returns the bodies as the entity compares them.
func comparedBodies[Req, Resp any](e entity[Req, Resp], desired, current Req) (Req, Req) {
	if c, ok := e.(comparer[Req]); ok {
		return c.Comparable(desired, current)
	}
	return desired, current
}

// plannedEntity returns the response a create of desired would give, with a placeholder ID,
// so that a plan can go on with the entities that depend on it.
func plannedEntity[Resp any](ctx context.Context, kind, name string, desired any) (*Resp, error) {
	id := migration.PlannedID(ctx, kind, name)
	if planned, err := decodeWithID[Resp](desired, id); err == nil {
		return planned, nil
	}
	// The request and the response type differ in a field: keep only the ID and the name.
	planned, err := decodeWithID[Resp](map[string]any{"name": name}, id)
	if err != nil {
		return nil, fmt.Errorf("planning %s %q: %w", kind, name, err)
	}
	return planned, nil
}

// decodeWithID converts a request body into a response with the given ID.
func decodeWithID[Resp any](body any, id string) (*Resp, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if unmarshalErr := json.Unmarshal(raw, &fields); unmarshalErr != nil {
		return nil, unmarshalErr
	}
	fields["id"] = id
	withID, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	planned := new(Resp)
	if decodeErr := json.Unmarshal(withID, planned); decodeErr != nil {
		return nil, decodeErr
	}
	return planned, nil
}
