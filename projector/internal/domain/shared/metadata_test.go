// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared_test

import (
	"github.com/telekom/controlplane/projector/internal/domain/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metadata", func() {
	Describe("NodeHash", func() {
		It("returns a 16-character hex string", func() {
			hash := shared.NodeHash("default", "my-zone")
			Expect(hash).To(HaveLen(16))
		})

		It("is deterministic", func() {
			h1 := shared.NodeHash("ns", "name")
			h2 := shared.NodeHash("ns", "name")
			Expect(h1).To(Equal(h2))
		})

		It("differs for different inputs", func() {
			h1 := shared.NodeHash("ns-a", "name")
			h2 := shared.NodeHash("ns-b", "name")
			Expect(h1).NotTo(Equal(h2))
		})

		It("differs when namespace and name are swapped", func() {
			h1 := shared.NodeHash("a", "b")
			h2 := shared.NodeHash("b", "a")
			Expect(h1).NotTo(Equal(h2))
		})
	})

	Describe("EnvironmentFromLabels", func() {
		It("returns the environment label value", func() {
			labels := map[string]string{
				shared.EnvironmentLabelKey: "production",
			}
			Expect(shared.EnvironmentFromLabels(labels)).To(Equal("production"))
		})

		It("returns empty string for nil labels", func() {
			Expect(shared.EnvironmentFromLabels(nil)).To(BeEmpty())
		})

		It("returns empty string when label is missing", func() {
			labels := map[string]string{"other": "value"}
			Expect(shared.EnvironmentFromLabels(labels)).To(BeEmpty())
		})

		It("returns empty string for empty labels map", func() {
			labels := map[string]string{}
			Expect(shared.EnvironmentFromLabels(labels)).To(BeEmpty())
		})
	})

	Describe("NewMetadata", func() {
		It("populates all fields correctly", func() {
			labels := map[string]string{
				shared.EnvironmentLabelKey: "staging",
			}
			m := shared.NewMetadata("my-ns", "my-name", labels)

			Expect(m.Namespace).To(Equal("my-ns"))
			Expect(m.Name).To(Equal("my-name"))
			Expect(m.Environment).To(Equal("staging"))
			Expect(m.NodeHash).To(HaveLen(16))
			Expect(m.NodeHash).To(Equal(shared.NodeHash("my-ns", "my-name")))
		})

		It("handles nil labels", func() {
			m := shared.NewMetadata("ns", "name", nil)
			Expect(m.Environment).To(BeEmpty())
			Expect(m.NodeHash).To(HaveLen(16))
		})
	})
})
