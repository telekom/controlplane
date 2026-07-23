// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCommand(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "xDS Management Command Suite")
}

var _ = Describe("Node mapping flags", func() {
	It("parses explicit node-to-target mappings", func() {
		mappings, err := parseMappings("node-a=target-a,node-b=target-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(mappings).To(Equal(map[string]string{"node-a": "target-a", "node-b": "target-a"}))
	})

	It("rejects malformed and duplicate mappings", func() {
		_, err := parseMappings("node-a")
		Expect(err).To(HaveOccurred())
		_, err = parseMappings("node-a=target-a,node-a=target-b")
		Expect(err).To(HaveOccurred())
	})
})
