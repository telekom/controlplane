// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

const (
	// plannedIDPrefix marks the ID of an entity that a plan would create. A backend never
	// returns it, so a lookup by it finds nothing.
	plannedIDPrefix = "planned--"
	// hashLength is the number of hex digits of a plan hash. 64 bits are enough to tell
	// the pending changes of one object apart.
	hashLength = 16
	// maxValueLength limits a diff value in the status. The hash covers the full value.
	maxValueLength = 1024
	// maxRecordedDiffs limits the diffs in the status. The hash covers all of them.
	maxRecordedDiffs = 100
)

// Operations of a Change.
const (
	OperationCreate = "create"
	OperationUpdate = "update"
	OperationDelete = "delete"
)

// Change is the change of one backend entity.
type Change struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Operation string `json:"operation"`
	Diffs     []Diff `json:"diffs,omitempty"`
}

// Plan collects the backend changes of one reconcile without writing them. The changes of
// one object are approved or blocked together.
type Plan struct {
	mu      sync.Mutex
	changes []Change
	planned map[string]string
}

// NewPlan returns an empty Plan.
func NewPlan() *Plan {
	return &Plan{planned: map[string]string{}}
}

type planKey struct{}

type applyKey struct{}

// WithPlan returns a ctx in which backend writes and deletes are recorded in plan instead
// of being executed.
func WithPlan(ctx context.Context, plan *Plan) context.Context {
	return context.WithValue(ctx, planKey{}, plan)
}

// WithApply returns a ctx in which backend writes and deletes are executed without a
// check, because the plan of the same reconcile was allowed or approved.
func WithApply(ctx context.Context) context.Context {
	return context.WithValue(ctx, applyKey{}, true)
}

func planFrom(ctx context.Context) *Plan {
	if plan, ok := ctx.Value(planKey{}).(*Plan); ok {
		return plan
	}
	return nil
}

func applying(ctx context.Context) bool {
	apply, ok := ctx.Value(applyKey{}).(bool)
	return ok && apply
}

// PlannedID registers an entity the plan in ctx would create and returns its placeholder
// ID. It returns "" if ctx has no plan.
func PlannedID(ctx context.Context, kind, name string) string {
	plan := planFrom(ctx)
	if plan == nil {
		return ""
	}
	id := plannedIDPrefix + kind + "--" + name
	plan.mu.Lock()
	defer plan.mu.Unlock()
	plan.planned[kind+"/"+name] = id
	plan.planned[kind+"/"+id] = id
	return id
}

// LookupPlanned returns the placeholder ID of an entity the plan in ctx would create,
// looked up by its name or ID.
func LookupPlanned(ctx context.Context, kind, nameOrID string) (string, bool) {
	if strings.HasPrefix(nameOrID, plannedIDPrefix) {
		return nameOrID, true
	}
	plan := planFrom(ctx)
	if plan == nil {
		return "", false
	}
	plan.mu.Lock()
	defer plan.mu.Unlock()
	id, ok := plan.planned[kind+"/"+nameOrID]
	return id, ok
}

// IsPlanned reports whether id is the placeholder ID of a planned entity.
func IsPlanned(id string) bool {
	return strings.HasPrefix(id, plannedIDPrefix)
}

func (p *Plan) add(change Change) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.changes = append(p.changes, change)
}

// Changes returns the recorded changes in the order they would be written.
func (p *Plan) Changes() []Change {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Change(nil), p.changes...)
}

// Hash identifies the changes of the plan, including the values of every diff. The same
// pending change gives the same hash in every reconcile; any other change gives another.
func (p *Plan) Hash() string {
	raw, err := json.Marshal(p.Changes())
	if err != nil {
		raw = []byte(err.Error())
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:hashLength]
}

// unexpected returns the number of diffs that are not on the ignore list.
func (p *Plan) unexpected() int {
	count := 0
	for _, change := range p.Changes() {
		for _, diff := range change.Diffs {
			if !diff.Ignored {
				count++
			}
		}
	}
	return count
}

// Status returns the plan as the pending migration status of an object. Created entities
// show their whole body, so that the reviewer sees what would be created.
func (p *Plan) Status(hash string, mode Mode) *types.MigrationStatus {
	changes, truncated := recordChanges(p.Changes(), false)
	return &types.MigrationStatus{Hash: hash, Mode: string(mode), Changes: changes, Truncated: truncated}
}

// Applied returns the plan as the record of an applied change. Created entities are
// recorded by the hash of their body, to keep the record small.
func (p *Plan) Applied(hash string, mode Mode, decision Decision, approvedHash string, generation int64, at time.Time) *types.AppliedChanges {
	changes, truncated := recordChanges(p.Changes(), true)
	return &types.AppliedChanges{
		Hash:         hash,
		AppliedAt:    metav1.NewTime(at),
		Generation:   generation,
		Mode:         string(mode),
		Decision:     string(decision),
		ApprovedHash: approvedHash,
		Changes:      changes,
		Truncated:    truncated,
	}
}

// recordChanges converts changes into status records within the size limits.
func recordChanges(changes []Change, hashCreates bool) ([]types.Change, bool) {
	records := make([]types.Change, 0, len(changes))
	truncated, diffs := false, 0
	for _, change := range changes {
		record := types.Change{Kind: change.Kind, Name: change.Name, Operation: change.Operation}
		for _, diff := range change.Diffs {
			if diffs == maxRecordedDiffs {
				truncated = true
				break
			}
			diffs++
			desired, cut := recordValue(diff.Desired)
			if hashCreates && change.Operation == OperationCreate && diff.Path == Root {
				desired, cut = recordValue(mask(diff.Desired))
			}
			current, cutCurrent := recordValue(diff.Current)
			truncated = truncated || cut || cutCurrent
			record.Diffs = append(record.Diffs, types.Diff{Path: diff.Path, Desired: desired, Current: current, Ignored: diff.Ignored})
		}
		records = append(records, record)
	}
	return records, truncated
}

// recordValue returns v as JSON, cut to maxValueLength, and whether it was cut.
func recordValue(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err.Error(), false
	}
	if len(raw) > maxValueLength {
		return string(raw[:maxValueLength]) + "...(truncated)", true
	}
	return string(raw), false
}
