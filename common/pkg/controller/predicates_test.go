// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
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
})
