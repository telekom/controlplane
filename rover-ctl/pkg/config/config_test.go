// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
)

var _ = Describe("Config", func() {
	BeforeEach(func() {
		viper.Reset()
		config.Initialize()
		GinkgoT().Setenv("RESOURCE_PATH", "")
		GinkgoT().Setenv("ROVER_RESOURCE_PATH", "")
	})

	AfterEach(func() {
		viper.Reset()
		config.Initialize()
	})

	Describe("ResourcePath", func() {
		It("should be empty when RESOURCE_PATH is empty", func() {
			GinkgoT().Setenv("RESOURCE_PATH", "")
			Expect(config.ResourcePath()).To(BeEmpty())
		})

		It("should read the unprefixed RESOURCE_PATH", func() {
			GinkgoT().Setenv("RESOURCE_PATH", "./resources")
			Expect(config.ResourcePath()).To(Equal("./resources"))
		})

		// AutomaticEnv is consulted before explicit bindings, so the prefixed
		// variable wins if set. Documented here to catch viper behavior changes.
		It("should prefer ROVER_RESOURCE_PATH via AutomaticEnv when set", func() {
			GinkgoT().Setenv("RESOURCE_PATH", "./resources")
			GinkgoT().Setenv("ROVER_RESOURCE_PATH", "./prefixed")
			Expect(config.ResourcePath()).To(Equal("./prefixed"))
		})

		It("should keep the ROVER prefix for other keys", func() {
			GinkgoT().Setenv("ROVER_LOG_LEVEL", "debug")
			Expect(viper.GetString("log.level")).To(Equal("debug"))
		})
	})
})
