// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

// Object is a Kubernetes object whose backend changes run through the change pipeline.
// The generated DeepCopy methods of a CRD type implement the copy methods.
type Object[T any] interface {
	client.Object
	DeepCopy() T
	DeepCopyInto(out T)
	SetMigrationStatus(status *types.MigrationStatus)
	SetLastAppliedChanges(applied *types.AppliedChanges)
}

// now is the clock of the applied-change records.
var now = time.Now

// Run runs the backend changes of one reconcile of obj, an object named by kind, through
// the gate. build reads the backend, computes the desired state and writes it for target.
//
//  1. Plan: build runs on a copy of obj and records its changes instead of writing them.
//  2. Decide: the gate decides on the changes as one bundle.
//  3. A blocked plan sets status.migration and returns a BlockedError. A plan without
//     changes keeps the result of the copy and writes nothing. Otherwise build runs again
//     on obj and writes, and status.lastAppliedChanges records what was applied.
//
// With a nil gate, build runs once on obj and writes directly.
func Run[T Object[T]](ctx context.Context, gate *Gate, obj T, kind string, build func(ctx context.Context, target T) error) error {
	if gate == nil {
		return build(ctx, obj)
	}
	obj.SetMigrationStatus(nil)

	plan := NewPlan()
	target := obj.DeepCopy()
	if err := build(WithPlan(ctx, plan), target); err != nil {
		return fmt.Errorf("planning %s: %w", kind, err)
	}

	decision, status, err := gate.Decide(ctx, plan, kind, obj)
	switch decision {
	case DecisionBlocked:
		obj.SetMigrationStatus(status)
		return err
	case DecisionUnchanged:
		// Nothing was written or created, so the plan holds no placeholders and its result
		// is the result of the reconcile.
		target.DeepCopyInto(obj)
		return nil
	}

	if err := build(WithApply(ctx), obj); err != nil {
		return err
	}
	hash := plan.Hash()
	approvedHash := ""
	if decision == DecisionApproved {
		approvedHash = hash
	}
	obj.SetLastAppliedChanges(plan.Applied(hash, gate.Mode(), decision, approvedHash, obj.GetGeneration(), now()))
	if recorder, ok := contextutil.RecoderFromContext(ctx); ok {
		recorder.Eventf(obj, "Normal", "ChangesApplied", "Applied %d backend change(s) with hash %s (%s, migration mode %s)",
			len(plan.Changes()), hash, decision, gate.Mode())
	}
	return nil
}
