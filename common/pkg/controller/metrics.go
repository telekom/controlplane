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

var eventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "controlplane_controller_source_events_total",
	Help: "Events observed per controller source, before predicate filtering and queue deduplication.",
}, []string{"controller", "role", "source", "verb"})

func init() {
	metrics.Registry.MustRegister(eventsTotal)
}

var _ predicate.Predicate = &countingPredicate{}

// Count returns a predicate that counts every event it observes and then
// delegates the filtering decision to inner. With no inner predicates it always
// returns true.
//
// Counting happens before inner runs, so events that inner rejects are still
// counted. Always pass filtering predicates to Count rather than listing them
// alongside it in builder.WithPredicates: that call ANDs its predicates and
// short-circuits on the first false, which would silently drop counts.
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

// observe records one event of the given verb for the object's type.
func (p *countingPredicate) observe(verb string, obj client.Object) {
	eventsTotal.WithLabelValues(p.controller, p.role, sourceOf(obj), verb).Inc()
}

// sourceOf returns the Kind of obj, e.g. "ConfigMap".
//
// It uses reflection rather than obj.GetObjectKind(): TypeMeta is empty on typed
// objects delivered by an informer, so the GVK there is blank. apiutil.GVKForObject
// would give the group-qualified GVK but needs a scheme and an error path, and no
// two watched Kinds in this repo share a name across groups.
func sourceOf(obj client.Object) string {
	if obj == nil {
		return "unknown"
	}
	t := reflect.TypeOf(obj)
	if t == nil {
		return "unknown"
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Name() == "" {
		return "unknown"
	}
	return t.Name()
}

func (p *countingPredicate) Create(e event.CreateEvent) bool {
	p.observe("create", e.Object)
	for _, i := range p.inner {
		if !i.Create(e) {
			return false
		}
	}
	return true
}

func (p *countingPredicate) Update(e event.UpdateEvent) bool {
	p.observe("update", e.ObjectNew)
	for _, i := range p.inner {
		if !i.Update(e) {
			return false
		}
	}
	return true
}

func (p *countingPredicate) Delete(e event.DeleteEvent) bool {
	p.observe("delete", e.Object)
	for _, i := range p.inner {
		if !i.Delete(e) {
			return false
		}
	}
	return true
}

func (p *countingPredicate) Generic(e event.GenericEvent) bool {
	p.observe("generic", e.Object)
	for _, i := range p.inner {
		if !i.Generic(e) {
			return false
		}
	}
	return true
}
