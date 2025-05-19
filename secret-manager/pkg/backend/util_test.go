// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

var _ = Describe("Util Tests", func() {
	BeforeEach(func() {

	})

	Context("Hashing", func() {
		It("should create a hex-encoded hash", func() {
			hash := backend.MakeChecksum("test")
			Expect(hash).To(Equal("9f86d081884c"))
		})
	})
})
