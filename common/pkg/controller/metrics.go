// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Roles describe which builder call a watch originates from.
const (
	RoleFor     = "for"
	RoleOwns    = "owns"
	RoleWatches = "watches"
)

// Results record what the source's predicates did with an observed event.
const (
	ResultPassed   = "passed"   // admitted by this source's predicates
	ResultFiltered = "filtered" // rejected by an inner predicate
	unknownSource  = "unknown"
)

var eventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "controlplane_controller_source_events_total",
	Help: "Events observed per controller source, labelled by whether the source's predicates admitted the event.",
}, []string{"controller", "role", "source", "verb", "result"})

func init() {
	metrics.Registry.MustRegister(eventsTotal)
}

var _ predicate.Predicate = &countingPredicate{}

// Count returns a predicate that delegates the filtering decision to inner and
// records every event it observes, labelled with that decision. With no inner
// predicates it always returns true.
//
// Counting happens after inner runs: the event is recorded either way, with the
// filtering outcome in the result label (ResultPassed or ResultFiltered). This is
// why filtering predicates must be passed to Count rather than listed alongside it
// in builder.WithPredicates: that call ANDs its predicates and short-circuits on
// the first false, so Count would never see — and so could not label — the
// decision another predicate made.
//
// The controller argument must be the primary Kind lowercased, e.g. "rover" and
// not "rover-controller". That is how controller-runtime labels its own metrics,
// and matching it is what allows this metric to be joined against
// workqueue_adds_total and controller_runtime_reconcile_total.
func Count(controller, role string, inner ...predicate.Predicate) predicate.Predicate {
	return &countingPredicate{
		controller: controller,
		role:       role,
		inner:      inner,
	}
}

// countingPredicate holds no per-event state, so one instance may be shared
// across several watches of the same controller and role.
type countingPredicate struct {
	controller string
	role       string
	inner      []predicate.Predicate
}

// observe records one event of the given verb, labelled with the filtering outcome.
func (p *countingPredicate) observe(verb string, obj client.Object, passed bool) {
	result := ResultFiltered
	if passed {
		result = ResultPassed
	}
	eventsTotal.WithLabelValues(p.controller, p.role, sourceOf(obj), verb, result).Inc()
}

// admit runs the inner predicates, ANDing them with short-circuit, and returns
// whether the event survives. No inner predicates means the event always passes.
func (p *countingPredicate) admit(eval func(predicate.Predicate) bool) bool {
	for _, i := range p.inner {
		if !eval(i) {
			return false
		}
	}
	return true
}

// sourceOf returns the Kind of obj, e.g. "ConfigMap".
//
// It uses reflection rather than obj.GetObjectKind(): TypeMeta is empty on typed
// objects delivered by an informer, so the GVK there is blank. apiutil.GVKForObject
// would give the group-qualified GVK but needs a scheme and an error path, and no
// two watched Kinds in this repo share a name across groups. That last part is an
// assumption that fails silently: adding a watch on a Kind whose name collides with
// another group's Kind will merge both into one series with no error, and switching
// sourceOf to apiutil.GVKForObject with a scheme is the upgrade path if that happens.
func sourceOf(obj client.Object) string {
	if obj == nil {
		return unknownSource
	}
	t := reflect.TypeOf(obj)
	if t == nil {
		return unknownSource
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Name() == "" {
		return unknownSource
	}
	return t.Name()
}

func (p *countingPredicate) Create(e event.CreateEvent) bool {
	passed := p.admit(func(i predicate.Predicate) bool { return i.Create(e) })
	p.observe("create", e.Object, passed)
	return passed
}

func (p *countingPredicate) Update(e event.UpdateEvent) bool {
	passed := p.admit(func(i predicate.Predicate) bool { return i.Update(e) })
	p.observe("update", e.ObjectNew, passed)
	return passed
}

func (p *countingPredicate) Delete(e event.DeleteEvent) bool {
	passed := p.admit(func(i predicate.Predicate) bool { return i.Delete(e) })
	p.observe("delete", e.Object, passed)
	return passed
}

func (p *countingPredicate) Generic(e event.GenericEvent) bool {
	passed := p.admit(func(i predicate.Predicate) bool { return i.Generic(e) })
	p.observe("generic", e.Object, passed)
	return passed
}
