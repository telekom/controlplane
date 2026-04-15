// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure_test

import (
	"github.com/telekom/controlplane/projector/internal/infrastructure"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EdgeCache", func() {
	var cache *infrastructure.EdgeCache

	BeforeEach(func() {
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cache.Close()
	})

	It("returns miss for unknown key", func() {
		val, found := cache.Get("zone", "nonexistent")
		Expect(found).To(BeFalse())
		Expect(val).To(Equal(0))
	})

	It("stores and retrieves a value", func() {
		cache.Set("zone", "zone-a", 42)
		cache.Wait() // ensure async Set is applied

		val, found := cache.Get("zone", "zone-a")
		Expect(found).To(BeTrue())
		Expect(val).To(Equal(42))
	})

	It("deletes a value", func() {
		cache.Set("zone", "zone-b", 99)
		cache.Wait()

		cache.Del("zone", "zone-b")

		val, found := cache.Get("zone", "zone-b")
		Expect(found).To(BeFalse())
		Expect(val).To(Equal(0))
	})

	It("isolates different entity types", func() {
		cache.Set("zone", "shared-name", 1)
		cache.Set("group", "shared-name", 2)
		cache.Wait()

		zoneVal, zoneFound := cache.Get("zone", "shared-name")
		groupVal, groupFound := cache.Get("group", "shared-name")

		Expect(zoneFound).To(BeTrue())
		Expect(zoneVal).To(Equal(1))
		Expect(groupFound).To(BeTrue())
		Expect(groupVal).To(Equal(2))
	})

	It("overwrites existing value", func() {
		cache.Set("zone", "zone-c", 10)
		cache.Wait()
		cache.Set("zone", "zone-c", 20)
		cache.Wait()

		val, found := cache.Get("zone", "zone-c")
		Expect(found).To(BeTrue())
		Expect(val).To(Equal(20))
	})

	Describe("NewEdgeCache", func() {
		It("succeeds with valid configuration", func() {
			c, err := infrastructure.NewEdgeCache(100_000, 10<<20, 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeNil())
			c.Close()
		})
	})
})
