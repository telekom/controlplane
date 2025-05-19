// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package hash

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHash(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hash Suite")
}

var spec = map[string]any{
	"foo":     "bar",
	"version": "0",
	"some": map[string]any{
		"nested": "value",
	},
}

var _ = Describe("Hash", func() {

	Context("ComputeHash", func() {
		It("should return a hash", func() {
			var i uint32 = 42
			hash := ComputeHash("content", &i)
			Expect(hash).ToNot(BeEmpty())
		})
	})

	Context("Collision", func() {

		var seenHashes = make(map[string]bool)

		It("should not have collisions", func() {
			for i := 0; i < 1000; i++ {
				spec["version"] = i
				hash := ComputeHash(&spec, nil)
				Expect(seenHashes[hash]).To(BeFalse())
				seenHashes[hash] = true
			}
		})
	})
})

func BenchmarkComputeHash(b *testing.B) {
	for n := 0; n < b.N; n++ {
		ComputeHash(&spec, nil)
	}
}
