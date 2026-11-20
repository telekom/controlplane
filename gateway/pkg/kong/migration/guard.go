// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package migration guards the Kong writes of the gateway operator while an earlier
// operator still owns the Kong configuration. In Migration Mode, every write and delete
// is compared with the current Kong state first. A difference that is neither on the
// ignore list nor approved blocks the write, so the switch to this operator cannot change
// Kong unnoticed. In strict mode, every write needs an approval.
package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// maxReportedPaths limits the paths named in the error, which ends up in a condition.
// status.migration and the log entry have all diffs.
const maxReportedPaths = 5

// Config is the Migration Mode configuration file.
type Config struct {
	// Ignore lists the diffs that are expected and may be written.
	Ignore []Rule `json:"ignore"`
}

// Rule ignores the diffs at Paths of the entities of one kind.
type Rule struct {
	// Kind is the Kong entity kind: route, service, upstream, target, plugin, consumer or
	// consumer_group.
	Kind string `json:"kind"`
	// Name optionally narrows the rule to entities whose name matches this regular
	// expression completely. The name is the Kong name of routes, services and upstreams,
	// the plugin name of plugins, the target of targets and the username of consumers.
	// Deletes are matched against the name or ID they are issued with.
	Name string `json:"name,omitempty"`
	// Paths are JSON paths such as "$.tags", "$.config.*" or "$.paths[*]". A path also
	// covers everything below it; "$" covers the whole entity, including its creation
	// and deletion.
	Paths []string `json:"paths"`
}

var knownKinds = []string{"route", "service", "upstream", "target", "plugin", "consumer", "consumer_group"}

type rule struct {
	kind  string
	name  *regexp.Regexp
	paths [][]string
}

// Guard compares Kong writes with the current state. A nil Guard allows everything.
type Guard struct {
	rules  []rule
	strict bool
}

var (
	diffsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gateway",
		Subsystem: "kong",
		Name:      "migration_diffs_total",
		Help:      "Kong entity diffs found in Migration Mode, by whether they were on the ignore list.",
	}, []string{"entity", "outcome"})
	checksTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gateway",
		Subsystem: "kong",
		Name:      "migration_checks_total",
		Help:      "Reconciles of Routes and Consumers checked in Migration Mode, by whether their Kong changes were unchanged, allowed, approved or blocked.",
	}, []string{"kind", "outcome"})
)

func init() {
	metrics.Registry.MustRegister(diffsTotal, checksTotal)
}

// Load reads the ignore list at path and returns a Guard for mode. ModeOff gives a nil
// Guard. An empty path gives a Guard without ignore rules, which blocks every diff.
func Load(path string, mode Mode) (*Guard, error) {
	if mode == ModeOff {
		return nil, nil
	}
	if path == "" {
		return New(Config{}, mode)
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator flag
	if err != nil {
		return nil, fmt.Errorf("reading migration config: %w", err)
	}
	cfg := Config{}
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing migration config: %w", err)
	}
	return New(cfg, mode)
}

// New validates cfg and returns a Guard for ModeOn or ModeStrict.
func New(cfg Config, mode Mode) (*Guard, error) {
	if mode != ModeOn && mode != ModeStrict {
		return nil, fmt.Errorf("a guard needs migration mode %q or %q, got %q", ModeOn, ModeStrict, mode)
	}
	g := &Guard{strict: mode == ModeStrict}
	for i, r := range cfg.Ignore {
		if !slices.Contains(knownKinds, r.Kind) {
			return nil, fmt.Errorf("ignore[%d]: unknown kind %q, expected one of %v", i, r.Kind, knownKinds)
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

// Mode returns the Migration Mode of the guard, ModeOff for a nil guard.
func (g *Guard) Mode() Mode {
	switch {
	case g == nil:
		return ModeOff
	case g.strict:
		return ModeStrict
	default:
		return ModeOn
	}
}

// CheckWrite checks a write of an entity and reports whether it may be executed now.
// current is ignored if exists is false: the write would create the entity.
//
// In a ctx with a plan (see WithPlan), the change is recorded and not executed. In a ctx of
// an allowed plan (see WithApply), it is executed. Otherwise it is checked on its own: a diff
// that is not ignored, or any diff in strict mode, returns a BlockedError, because an
// approval needs the plan of the whole Route or Consumer.
func (g *Guard) CheckWrite(ctx context.Context, kind, name string, desired, current any, exists bool) (bool, error) {
	if g == nil || applying(ctx) {
		return true, nil
	}
	change := Change{Kind: kind, Name: name, Operation: OperationUpdate}
	if exists {
		diffs, err := Compare(desired, current)
		if err != nil {
			return false, fmt.Errorf("comparing %s %q in migration mode: %w", kind, name, err)
		}
		change.Diffs = diffs
	} else {
		value, err := canonical(desired)
		if err != nil {
			return false, fmt.Errorf("serializing %s %q in migration mode: %w", kind, name, err)
		}
		change.Operation = OperationCreate
		change.Diffs = []Diff{{Path: Root, Desired: value}}
	}
	return g.check(ctx, change)
}

// CheckDelete checks the deletion of an entity like CheckWrite.
func (g *Guard) CheckDelete(ctx context.Context, kind, idOrName string) (bool, error) {
	if g == nil || applying(ctx) {
		return true, nil
	}
	return g.check(ctx, Change{Kind: kind, Name: idOrName, Operation: OperationDelete, Diffs: []Diff{{Path: Root, Current: idOrName}}})
}

func (g *Guard) check(ctx context.Context, change Change) (bool, error) {
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
	if unexpected == 0 && !g.strict {
		return true, nil
	}
	logr.FromContextOrDiscard(ctx).Info("Migration mode blocked unplanned kong write", "change", render(change))
	return false, ctrlerrors.BlockedErrorf("migration mode: %s of %s %q is not part of a planned reconcile and cannot be approved",
		change.Operation, change.Kind, change.Name)
}

// Decide decides on the plan of one reconcile of obj, a Route or Consumer named by kind.
// A plan without changes is allowed. In mode on, a plan whose diffs are all ignored is
// allowed. Otherwise the plan needs the approval of its hash on obj (see ApprovalAnnotation).
// For a blocked plan, Decide returns the status to show on obj and a BlockedError.
func (g *Guard) Decide(ctx context.Context, plan *Plan, kind string, obj metav1.Object) (*gatewayv1.MigrationStatus, error) {
	if g == nil || plan == nil {
		return nil, nil
	}
	changes := plan.Changes()
	if len(changes) == 0 {
		checksTotal.WithLabelValues(kind, "unchanged").Inc()
		return nil, nil
	}
	unexpected := plan.unexpected()
	for _, change := range changes {
		for _, diff := range change.Diffs {
			outcome := "unexpected"
			if diff.Ignored {
				outcome = "ignored"
			}
			diffsTotal.WithLabelValues(change.Kind, outcome).Inc()
		}
	}

	log := logr.FromContextOrDiscard(ctx)
	if unexpected == 0 && !g.strict {
		checksTotal.WithLabelValues(kind, "allowed").Inc()
		log.V(1).Info("Migration mode allowed kong changes with expected diffs", "changes", render(changes))
		return nil, nil
	}

	hash := plan.Hash()
	if IsApproved(obj, hash) {
		checksTotal.WithLabelValues(kind, "approved").Inc()
		log.Info("Migration mode applied approved kong changes", "hash", hash, "changes", render(changes))
		return nil, nil
	}

	checksTotal.WithLabelValues(kind, "blocked").Inc()
	log.Info("Migration mode blocked kong changes", "hash", hash, "changes", render(changes))
	hint := fmt.Sprintf("review status.migration and approve with the annotation %s=%q", ApprovalAnnotation, hash)
	if unexpected == 0 {
		return plan.Status(hash, g.Mode()), ctrlerrors.BlockedErrorf("migration mode strict: %d kong change(s) need approval; %s",
			len(changes), hint)
	}
	return plan.Status(hash, g.Mode()), ctrlerrors.BlockedErrorf("migration mode: %d kong change(s) have %d unexpected diff(s): %s; %s",
		len(changes), unexpected, summarize(changes), hint)
}

func (g *Guard) ignores(kind, name, path string) bool {
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
