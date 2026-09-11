// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// rejectAll is a predicate that filters out every event.
type rejectAll struct{}

func (rejectAll) Create(event.CreateEvent) bool   { return false }
func (rejectAll) Delete(event.DeleteEvent) bool   { return false }
func (rejectAll) Update(event.UpdateEvent) bool   { return false }
func (rejectAll) Generic(event.GenericEvent) bool { return false }

var _ = Describe("Count predicate", func() {
	// readCounter gathers the metric from the controller-runtime registry and
	// returns the value for the given label set, or 0 if the series is absent.
	readCounter := func(controller, role, source, verb, result string) float64 {
		families, err := metrics.Registry.Gather()
		Expect(err).ToNot(HaveOccurred())
		for _, f := range families {
			if f.GetName() != "controlplane_controller_source_events_total" {
				continue
			}
			for _, m := range f.GetMetric() {
				want := map[string]string{
					"controller": controller,
					"role":       role,
					"source":     source,
					"verb":       verb,
					"result":     result,
				}
				match := true
				for _, l := range m.GetLabel() {
					if want[l.GetName()] != l.GetValue() {
						match = false
						break
					}
				}
				if match {
					return m.GetCounter().GetValue()
				}
			}
		}
		return 0
	}

	obj := &corev1.ConfigMap{}

	It("counts each verb once and passes events through with no inner predicate", func() {
		p := Count("testctl", RoleWatches)

		before := map[string]float64{}
		for _, verb := range []string{"create", "update", "delete", "generic"} {
			before[verb] = readCounter("testctl", RoleWatches, "ConfigMap", verb, ResultPassed)
		}

		Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
		Expect(p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj})).To(BeTrue())
		Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
		Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())

		for _, verb := range []string{"create", "update", "delete", "generic"} {
			Expect(readCounter("testctl", RoleWatches, "ConfigMap", verb, ResultPassed)).
				To(Equal(before[verb]+1), "verb %s", verb)
		}
	})

	It("counts events that the inner predicate rejects, and still rejects them", func() {
		p := Count("filterctl", RoleOwns, rejectAll{})

		before := readCounter("filterctl", RoleOwns, "ConfigMap", "create", ResultFiltered)

		Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())

		Expect(readCounter("filterctl", RoleOwns, "ConfigMap", "create", ResultFiltered)).
			To(Equal(before + 1))
	})

	It("records filtered and passed events under separate result labels", func() {
		rejected := Count("splitctl", RoleWatches, rejectAll{})
		admitted := Count("splitctl", RoleWatches)

		beforeFiltered := readCounter("splitctl", RoleWatches, "ConfigMap", "create", ResultFiltered)
		beforePassed := readCounter("splitctl", RoleWatches, "ConfigMap", "create", ResultPassed)

		Expect(rejected.Create(event.CreateEvent{Object: obj})).To(BeFalse())

		Expect(readCounter("splitctl", RoleWatches, "ConfigMap", "create", ResultFiltered)).
			To(Equal(beforeFiltered + 1))
		Expect(readCounter("splitctl", RoleWatches, "ConfigMap", "create", ResultPassed)).
			To(Equal(beforePassed), "a filtered event must not count as passed")

		Expect(admitted.Create(event.CreateEvent{Object: obj})).To(BeTrue())

		Expect(readCounter("splitctl", RoleWatches, "ConfigMap", "create", ResultPassed)).
			To(Equal(beforePassed + 1))
		Expect(readCounter("splitctl", RoleWatches, "ConfigMap", "create", ResultFiltered)).
			To(Equal(beforeFiltered+1), "an admitted event must not count as filtered")
	})

	It("derives the source label from the object type", func() {
		p := Count("srcctl", RoleFor)

		before := readCounter("srcctl", RoleFor, "Secret", "create", ResultPassed)
		Expect(p.Create(event.CreateEvent{Object: &corev1.Secret{}})).To(BeTrue())
		Expect(readCounter("srcctl", RoleFor, "Secret", "create", ResultPassed)).To(Equal(before + 1))
	})

	It("ANDs multiple inner predicates", func() {
		firstRan := false
		p := Count("andctl", RoleWatches, predicate.NewPredicateFuncs(func(client.Object) bool {
			firstRan = true
			return true
		}), rejectAll{})

		Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
		Expect(firstRan).To(BeTrue(), "the first predicate must have been consulted")
	})

	It("tolerates a nil object without panicking", func() {
		p := Count("nilctl", RoleWatches)
		Expect(func() { p.Create(event.CreateEvent{}) }).ToNot(Panic())
		Expect(func() { p.Update(event.UpdateEvent{}) }).ToNot(Panic())
	})
})
