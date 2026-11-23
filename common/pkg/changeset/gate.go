// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package changeset is the change pipeline of the operators that configure a backend
// (Kong, Keycloak, Horizon). A reconcile first plans its backend changes without writing
// them, then decides on them as one bundle and only then applies them. The gate decides:
// in Migration Mode, a change with a difference that is neither on the ignore list nor
// approved is blocked, so the takeover from an earlier operator cannot change the backend
// unnoticed. Every applied change is recorded on the object. See docs/adrs/0006 to 0014.
package changeset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
)

// maxReportedPaths limits the paths named in the error, which ends up in a condition.
// The status and the log entry have all diffs.
const maxReportedPaths = 5

// Decision is the outcome of a gate decision on a plan.
type Decision string

const (
	// DecisionUnchanged means the plan has no changes; nothing is written.
	DecisionUnchanged Decision = "unchanged"
	// DecisionAllowed means the plan may be applied without approval.
	DecisionAllowed Decision = "allowed"
	// DecisionApproved means the plan was approved by its hash.
	DecisionApproved Decision = "approved"
	// DecisionBlocked means the plan needs an approval.
	DecisionBlocked Decision = "blocked"
)

// Config is the ignore list file.
type Config struct {
	// Ignore lists the diffs that are expected and may be written.
	Ignore []Rule `json:"ignore"`
}

// Rule ignores the diffs at Paths of the entities of one kind.
type Rule struct {
	// Kind is a backend entity kind registered by the operator, such as route or client.
	Kind string `json:"kind"`
	// Name optionally narrows the rule to entities whose name matches this regular
	// expression completely.
	Name string `json:"name,omitempty"`
	// Paths are JSON paths such as "$.tags", "$.config.*" or "$.paths[*]". A path also
	// covers everything below it; "$" covers the whole entity, including its creation
	// and deletion.
	Paths []string `json:"paths"`
}

// Options configure a Gate.
type Options struct {
	// Operator names the operator in metrics, such as gateway or identity.
	Operator string
	// Mode is the Migration Mode.
	Mode Mode
	// Kinds are the entity kinds of the backend. The ignore list may only name these.
	Kinds []string
}

type rule struct {
	kind  string
	name  *regexp.Regexp
	paths [][]string
}

// Gate decides whether the backend changes of a reconcile are written. A nil Gate writes
// everything directly, without a plan.
type Gate struct {
	operator string
	mode     Mode
	rules    []rule
}

// Load reads the ignore list at path and returns a Gate. An empty path gives a Gate
// without ignore rules: in Migration Mode, every diff then needs an approval.
func Load(opts Options, path string) (*Gate, error) {
	if path == "" {
		return New(opts, Config{})
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator flag
	if err != nil {
		return nil, fmt.Errorf("reading migration config: %w", err)
	}
	cfg := Config{}
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing migration config: %w", err)
	}
	return New(opts, cfg)
}

// New validates cfg against the kinds of opts and returns a Gate.
func New(opts Options, cfg Config) (*Gate, error) {
	if _, err := ParseMode(string(opts.Mode)); err != nil {
		return nil, err
	}
	g := &Gate{operator: opts.Operator, mode: opts.Mode}
	for i, r := range cfg.Ignore {
		if !slices.Contains(opts.Kinds, r.Kind) {
			return nil, fmt.Errorf("ignore[%d]: unknown kind %q, expected one of %v", i, r.Kind, opts.Kinds)
		}
		if len(r.Paths) == 0 {
			return nil, fmt.Errorf("ignore[%d]: paths must not be empty", i)
		}
		compiled := rule{kind: r.Kind}
		if r.Name != "" {
			re, err := regexp.Compile("^(?:" + r.Name + ")$")
			if err != nil {
				return nil, fmt.Errorf("ignore[%d]: invalid name pattern: %w", i, err)
			}
			compiled.name = re
		}
		for _, p := range r.Paths {
			segments, err := parsePath(p)
			if err != nil {
				return nil, fmt.Errorf("ignore[%d]: %w", i, err)
			}
			compiled.paths = append(compiled.paths, segments)
		}
		g.rules = append(g.rules, compiled)
	}
	return g, nil
}

// Mode returns the Migration Mode of the gate, ModeOff for a nil gate.
func (g *Gate) Mode() Mode {
	if g == nil {
		return ModeOff
	}
	return g.mode
}

// CheckWrite checks a write of an entity and reports whether it may be executed now.
// current is ignored if exists is false: the write would create the entity. The maskers
// replace secret values before the comparison.
//
// In a ctx with a plan (see WithPlan), the change is recorded and not executed. In a ctx of
// an allowed plan (see WithApply), it is executed. Otherwise it is checked on its own: in
// Migration Mode, a diff that is not ignored, or any diff in strict mode, returns a
// BlockedError, because an approval needs the plan of the whole object.
func (g *Gate) CheckWrite(ctx context.Context, kind, name string, desired, current any, exists bool, maskers ...Masker) (bool, error) {
	if g == nil || applying(ctx) {
		return true, nil
	}
	change := Change{Kind: kind, Name: name, Operation: OperationUpdate}
	if exists {
		diffs, err := Compare(desired, current, maskers...)
		if err != nil {
			return false, fmt.Errorf("comparing %s %q: %w", kind, name, err)
		}
		change.Diffs = diffs
	} else {
		value, err := canonical(desired)
		if err != nil {
			return false, fmt.Errorf("serializing %s %q: %w", kind, name, err)
		}
		for _, mask := range maskers {
			value = mask(value)
		}
		change.Operation = OperationCreate
		change.Diffs = []Diff{{Path: Root, Desired: value}}
	}
	return g.check(ctx, change)
}

// CheckDelete checks the deletion of an entity like CheckWrite.
func (g *Gate) CheckDelete(ctx context.Context, kind, idOrName string) (bool, error) {
	if g == nil || applying(ctx) {
		return true, nil
	}
	return g.check(ctx, Change{Kind: kind, Name: idOrName, Operation: OperationDelete, Diffs: []Diff{{Path: Root, Current: idOrName}}})
}

func (g *Gate) check(ctx context.Context, change Change) (bool, error) {
	unexpected := 0
	for i := range change.Diffs {
		change.Diffs[i].Ignored = g.ignores(change.Kind, change.Name, change.Diffs[i].Path)
		if !change.Diffs[i].Ignored {
			unexpected++
		}
	}
	if plan := planFrom(ctx); plan != nil {
		plan.add(change)
		return false, nil
	}
	if g.mode == ModeOff || (unexpected == 0 && g.mode == ModeOn) {
		return true, nil
	}
	logr.FromContextOrDiscard(ctx).Info("Migration mode blocked unplanned backend write", "change", render(change))
	return false, ctrlerrors.BlockedErrorf("migration mode: %s of %s %q is not part of a planned reconcile and cannot be approved",
		change.Operation, change.Kind, change.Name)
}

// Decide decides on the plan of one reconcile of obj, an object named by kind. A plan
// without changes is unchanged. In mode off, every plan is allowed; in mode on, a plan whose
// diffs are all ignored is allowed. Otherwise the plan needs the approval of its hash on obj
// (see ApprovalAnnotation). For a blocked plan, Decide returns the status to show on obj and
// a BlockedError.
func (g *Gate) Decide(ctx context.Context, plan *Plan, kind string, obj metav1.Object) (Decision, *types.MigrationStatus, error) {
	changes := plan.Changes()
	if len(changes) == 0 {
		g.count(kind, DecisionUnchanged)
		return DecisionUnchanged, nil, nil
	}
	unexpected := plan.unexpected()
	for _, change := range changes {
		for _, diff := range change.Diffs {
			outcome := "unexpected"
			if diff.Ignored {
				outcome = "ignored"
			}
			diffsTotal.WithLabelValues(g.operator, change.Kind, outcome).Inc()
		}
	}

	log := logr.FromContextOrDiscard(ctx)
	if g.mode == ModeOff || (unexpected == 0 && g.mode == ModeOn) {
		g.count(kind, DecisionAllowed)
		log.V(1).Info("Backend changes allowed", "mode", g.mode, "changes", render(changes))
		return DecisionAllowed, nil, nil
	}

	hash := plan.Hash()
	if IsApproved(obj, hash) {
		g.count(kind, DecisionApproved)
		log.Info("Migration mode applies approved backend changes", "hash", hash, "changes", render(changes))
		return DecisionApproved, nil, nil
	}

	g.count(kind, DecisionBlocked)
	log.Info("Migration mode blocked backend changes", "hash", hash, "changes", render(changes))
	hint := fmt.Sprintf("review status.migration and approve with the annotation %s=%q", ApprovalAnnotation, hash)
	status := plan.Status(hash, g.mode)
	if unexpected == 0 {
		return DecisionBlocked, status, ctrlerrors.BlockedErrorf("migration mode strict: %d backend change(s) need approval; %s",
			len(changes), hint)
	}
	return DecisionBlocked, status, ctrlerrors.BlockedErrorf("migration mode: %d backend change(s) have %d unexpected diff(s): %s; %s",
		len(changes), unexpected, summarize(changes), hint)
}

func (g *Gate) count(kind string, decision Decision) {
	checksTotal.WithLabelValues(g.operator, kind, string(decision)).Inc()
}

func (g *Gate) ignores(kind, name, path string) bool {
	segments, err := parsePath(path)
	if err != nil {
		return false
	}
	for _, r := range g.rules {
		if r.kind != kind || (r.name != nil && !r.name.MatchString(name)) {
			continue
		}
		for _, pattern := range r.paths {
			if covers(pattern, segments) {
				return true
			}
		}
	}
	return false
}

func summarize(changes []Change) string {
	var paths []string
	total := 0
	for _, change := range changes {
		for _, diff := range change.Diffs {
			if diff.Ignored {
				continue
			}
			total++
			if len(paths) < maxReportedPaths {
				paths = append(paths, fmt.Sprintf("%s %q %s", change.Kind, change.Name, diff.Path))
			}
		}
	}
	if total > maxReportedPaths {
		paths = append(paths, fmt.Sprintf("and %d more", total-maxReportedPaths))
	}
	return strings.Join(paths, ", ")
}

func render(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(raw)
}
