// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
)

// Entity describes how a single backend entity is read, projected back into its request
// representation and written. Req is the request-body type, Resp the response type. The
// request types can be large, so the desired body is passed to Write by pointer.
type Entity[Req, Resp any] interface {
	// Kind is the entity kind used in the ignore list and the change records, such as route.
	Kind() string
	Get(ctx context.Context) (current *Resp, found bool, err error)
	Project(current *Resp) (Req, error)
	Write(ctx context.Context, desired *Req) (*Resp, error)
}

// Comparer is an optional capability of an Entity. Entities whose request type carries
// fields the operator does not manage implement it to exclude those fields, so that a
// backend default can never be mistaken for a change. It returns the desired and current
// bodies as they are compared and recorded.
type Comparer[Req any] interface {
	Comparable(desired, current Req) (Req, Req)
}

// Namer is an optional capability of an Entity that names it in the change records and
// for name rules of the ignore list. Without it, NameOf names the desired body.
type Namer[Req any] interface {
	NameOf(desired Req) string
}

// SecretMasker is an optional capability of an Entity whose bodies carry secrets. Its
// Masker replaces them before they are diffed and recorded.
type SecretMasker interface {
	Masker() Masker
}

// Outcome tells what Reconcile did.
type Outcome string

const (
	// OutcomeUnchanged means the backend already held the desired entity.
	OutcomeUnchanged Outcome = "unchanged"
	// OutcomeWritten means the entity was written.
	OutcomeWritten Outcome = "written"
	// OutcomePlanned means the write was recorded in a plan.
	OutcomePlanned Outcome = "planned"
	// OutcomeBlocked means the gate blocked the write.
	OutcomeBlocked Outcome = "blocked"
	// OutcomeError means reading or writing failed.
	OutcomeError Outcome = "error"
)

// Reconcile writes desired to the backend only if it differs from what the backend holds,
// and only as far as gate allows (see Gate.CheckWrite). It returns the current, written or,
// in a plan, the planned entity. A planned create returns desired with a placeholder ID
// (see PlannedID), so that the entities that depend on it can be planned too.
func Reconcile[Req, Resp any](ctx context.Context, gate *Gate, e Entity[Req, Resp], desired Req) (*Resp, Outcome, error) {
	current, found, err := e.Get(ctx)
	if err != nil {
		return nil, OutcomeError, err
	}

	compared := desired
	var projected Req
	if found {
		var projectErr error
		projected, projectErr = e.Project(current)
		if projectErr != nil {
			return nil, OutcomeError, projectErr
		}
		compared, projected = comparedBodies(e, desired, projected)
		if reflect.DeepEqual(compared, projected) {
			return current, OutcomeUnchanged, nil
		}
	}

	name := entityName(e, desired)
	write, err := gate.CheckWrite(ctx, e.Kind(), name, compared, projected, found, maskers(e)...)
	if err != nil {
		return nil, OutcomeBlocked, err
	}
	if !write {
		if found {
			return current, OutcomePlanned, nil
		}
		planned, plannedErr := plannedEntity[Resp](ctx, e.Kind(), name, desired)
		if plannedErr != nil {
			return nil, OutcomeError, plannedErr
		}
		return planned, OutcomePlanned, nil
	}

	logr.FromContextOrDiscard(ctx).V(1).Info("Writing backend entity", "entity", e.Kind(), "name", name, "exists", found)
	result, err := e.Write(ctx, &desired)
	if err != nil {
		return nil, OutcomeError, err
	}
	return result, OutcomeWritten, nil
}

// Delete deletes an entity through del as far as gate allows (see Gate.CheckDelete).
func Delete(ctx context.Context, gate *Gate, kind, idOrName string, del func(ctx context.Context) error) error {
	write, err := gate.CheckDelete(ctx, kind, idOrName)
	if err != nil || !write {
		return err
	}
	return del(ctx)
}

// comparedBodies returns the bodies as the entity compares them.
func comparedBodies[Req, Resp any](e Entity[Req, Resp], desired, current Req) (Req, Req) {
	if c, ok := e.(Comparer[Req]); ok {
		return c.Comparable(desired, current)
	}
	return desired, current
}

func entityName[Req, Resp any](e Entity[Req, Resp], desired Req) string {
	if n, ok := e.(Namer[Req]); ok {
		return n.NameOf(desired)
	}
	return NameOf(desired)
}

func maskers[Req, Resp any](e Entity[Req, Resp]) []Masker {
	if m, ok := e.(SecretMasker); ok {
		return []Masker{m.Masker()}
	}
	return nil
}

// plannedEntity returns the response a create of desired would give, with a placeholder ID.
func plannedEntity[Resp any](ctx context.Context, kind, name string, desired any) (*Resp, error) {
	id := PlannedID(ctx, kind, name)
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
