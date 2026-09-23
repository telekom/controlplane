// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func readyRoute(ready bool, consumers ...string) *gatewayv1.Route {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
		Status: gatewayv1.RouteStatus{
			Consumers: consumers,
			Conditions: []metav1.Condition{{
				Type:   condition.ConditionTypeReady,
				Status: status,
				Reason: "Testing",
			}},
		},
	}
}

var _ = Describe("RouteRelevantForConsumeRoutePredicate", func() {
	p := RouteRelevantForConsumeRoutePredicate{}

	update := func(old, updated *gatewayv1.Route) event.UpdateEvent {
		return event.UpdateEvent{ObjectOld: old, ObjectNew: updated}
	}

	Context("when a field the ConsumeRoute handler reads changes", func() {
		It("admits a change of the Ready condition", func() {
			Expect(p.Update(update(readyRoute(false), readyRoute(true)))).To(BeTrue())
			Expect(p.Update(update(readyRoute(true), readyRoute(false)))).To(BeTrue())
		})

		It("admits a consumer being added", func() {
			Expect(p.Update(update(readyRoute(true, "a"), readyRoute(true, "a", "b")))).To(BeTrue())
		})

		It("admits a consumer being removed", func() {
			Expect(p.Update(update(readyRoute(true, "a", "b"), readyRoute(true, "b")))).To(BeTrue())
		})
	})

	Context("when nothing the ConsumeRoute handler reads changes", func() {
		It("drops an update where status is identical", func() {
			Expect(p.Update(update(readyRoute(true, "a"), readyRoute(true, "a")))).To(BeFalse())
		})

		It("drops a change of Status.Properties", func() {
			old := readyRoute(true, "a")
			updated := readyRoute(true, "a")
			updated.Status.Properties = map[string]string{"some": "property"}

			Expect(p.Update(update(old, updated))).To(BeFalse())
		})

		It("drops a spec change", func() {
			old := readyRoute(true, "a")
			updated := readyRoute(true, "a")
			updated.Spec.PassThrough = true
			updated.Generation = 2

			Expect(p.Update(update(old, updated))).To(BeFalse())
		})

		It("drops a change of a condition other than Ready", func() {
			old := readyRoute(true, "a")
			updated := readyRoute(true, "a")
			updated.Status.Conditions = append(updated.Status.Conditions, metav1.Condition{
				Type:   condition.ConditionTypeProcessing,
				Status: metav1.ConditionTrue,
				Reason: "Testing",
			})

			Expect(p.Update(update(old, updated))).To(BeFalse())
		})
	})

	Context("when the event does not carry Routes", func() {
		It("admits the event rather than dropping it silently", func() {
			e := event.UpdateEvent{
				ObjectOld: &corev1.ConfigMap{},
				ObjectNew: &corev1.ConfigMap{},
			}
			Expect(p.Update(e)).To(BeTrue())
		})
	})

	Context("other event types", func() {
		It("admits creates, deletes and generic events", func() {
			Expect(p.Create(event.CreateEvent{Object: readyRoute(true)})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: readyRoute(true)})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{Object: readyRoute(true)})).To(BeTrue())
		})
	})
})
