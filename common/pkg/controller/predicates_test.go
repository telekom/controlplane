// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SkipInitialListPredicate", func() {
	p := SkipInitialListPredicate{}
	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"}}

	It("drops the synthetic creates of the initial cache sync", func() {
		Expect(p.Create(event.CreateEvent{Object: obj, IsInInitialList: true})).To(BeFalse())
	})

	It("admits a real create", func() {
		Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
	})

	It("admits update, delete and generic events", func() {
		Expect(p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj})).To(BeTrue())
		Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
		Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
	})

	It("records skipped initial-list creates as filtered when wrapped in Count", func() {
		counted := Count("skipinitialctl", RoleWatches, p)

		Expect(counted.Create(event.CreateEvent{Object: obj, IsInInitialList: true})).To(BeFalse())
		Expect(counted.Create(event.CreateEvent{Object: obj})).To(BeTrue())

		Expect(testutil.ToFloat64(eventsTotal.WithLabelValues("skipinitialctl", RoleWatches, "ConfigMap", "create", ResultFiltered))).To(Equal(1.0))
		Expect(testutil.ToFloat64(eventsTotal.WithLabelValues("skipinitialctl", RoleWatches, "ConfigMap", "create", ResultPassed))).To(Equal(1.0))
	})
})
