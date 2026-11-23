// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/changeset"
)

// Kong entity kinds of the change pipeline.
const (
	kindRoute         = parentRoute
	kindService       = "service"
	kindUpstream      = "upstream"
	kindTarget        = "target"
	kindPlugin        = "plugin"
	kindConsumer      = parentConsumer
	kindConsumerGroup = "consumer_group"
)

// EntityKinds are the Kong entity kinds of the change pipeline. The migration ignore list
// may only name these.
var EntityKinds = []string{kindRoute, kindService, kindUpstream, kindTarget, kindPlugin, kindConsumer, kindConsumerGroup}

// entity describes how a single Kong entity is read, projected back into its request
// representation and written. Req is the generated request-body type, Resp the generated
// response type.
type entity[Req, Resp any] = changeset.Entity[Req, Resp]

// comparer is an optional capability of an entity. Entities whose request type carries
// fields the controller does not manage implement it to exclude those fields, so that a
// Kong-side default can never be mistaken for a change.
type comparer[Req any] = changeset.Comparer[Req]

// reconcile writes desired to Kong only if it differs from what Kong already holds, and only
// as far as the gate allows. It returns the current, written or planned entity and whether a
// write happened.
func reconcile[Req, Resp any](ctx context.Context, gate *changeset.Gate, e entity[Req, Resp], desired Req) (*Resp, bool, error) {
	result, outcome, err := changeset.Reconcile(ctx, gate, e, desired)
	kongReconcileTotal.WithLabelValues(e.Kind(), string(outcome)).Inc()
	return result, outcome == changeset.OutcomeWritten, err
}
