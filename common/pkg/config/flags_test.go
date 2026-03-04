// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func withArgs(args []string, fn func()) {
	original := os.Args
	os.Args = args
	DeferCleanup(func() { os.Args = original })
	fn()
}

var _ = Describe("Flag-based config loading", func() {
	Context("parseConfigPath", func() {
		It("returns the default path when no flag is provided", func() {
			withArgs([]string{"controller"}, func() {
				path, err := parseConfigPath()
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(DefaultConfigPath))
			})
		})

		It("returns the custom path when --config is given", func() {
			customPath := "C:/tmp/custom.yaml"
			withArgs([]string{"controller", "--config", customPath}, func() {
				path, err := parseConfigPath()
				Expect(err).ToNot(HaveOccurred())
				Expect(path).To(Equal(customPath))
			})
		})
	})

	Context("Load helpers propagate flag errors", func() {
		It("propagates errors from LoadController", func() {
			withArgs([]string{"controller", "--unknown"}, func() {
				_, err := Load[EmptySpec]()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
