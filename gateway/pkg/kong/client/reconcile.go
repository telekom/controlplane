// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
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
// fields, so that a Kong-side default can never be mistaken for a change.
type comparer[Req any] interface {
	Equal(desired, current Req) bool
}

// reconcile writes desired to Kong only if it differs from what Kong already
// holds. It returns the current or written entity and whether a write happened.
func reconcile[Req, Resp any](ctx context.Context, e entity[Req, Resp], desired Req) (result *Resp, written bool, err error) {
	outcome := "error"
	defer func() { kongReconcileTotal.WithLabelValues(e.Name(), outcome).Inc() }()

	current, found, err := e.Get(ctx)
	if err != nil {
		return nil, false, err
	}

	if found {
		projected, projectErr := e.Project(current)
		if projectErr != nil {
			return nil, false, projectErr
		}
		if equal(e, desired, projected) {
			outcome = "unchanged"
			return current, false, nil
		}
	}

	logr.FromContextOrDiscard(ctx).V(1).Info("writing kong entity", "entity", e.Name(), "exists", found)
	result, err = e.Write(ctx, &desired)
	if err != nil {
		return nil, false, err
	}
	outcome = "written"
	return result, true, nil
}

func equal[Req, Resp any](e entity[Req, Resp], desired, current Req) bool {
	if c, ok := e.(comparer[Req]); ok {
		return c.Equal(desired, current)
	}
	return reflect.DeepEqual(desired, current)
}
