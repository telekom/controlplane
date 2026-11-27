// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package module

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

// Compile-time check: TypedModule implements Module interface.
var _ Module = &TypedModule[*corev1.ConfigMap, any, string]{}

var _ = Describe("TypedModule", func() {
	Describe("Name", func() {
		It("returns the configured module name", func() {
			m := &TypedModule[*corev1.ConfigMap, any, string]{
				ModuleName: "test-resource",
			}
			Expect(m.Name()).To(Equal("test-resource"))
		})
	})
})
