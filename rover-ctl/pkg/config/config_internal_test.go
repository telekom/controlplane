// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("bindEnvOrDie", func() {
	It("should panic with binding context when BindEnv fails", func() {
		Expect(func() { bindEnvOrDie() }).To(PanicWith(And(
			ContainSubstring("binding env []"),
			ContainSubstring("missing key to bind to"),
		)))
	})
})
