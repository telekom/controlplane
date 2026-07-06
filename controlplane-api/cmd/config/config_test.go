// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/controlplane-api/cmd/config"
)

var _ = Describe("SecurityConfig", func() {

	Describe("DefaultConfig", func() {
		It("defaults to jwt mode (secure by default)", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Security.Mode).To(Equal(security.ModeJWT))
		})
	})

	Describe("ReadConfig", func() {
		readConfig := func(yaml string) *config.ServerConfig {
			cfg, err := config.ReadConfig(strings.NewReader(yaml))
			Expect(err).NotTo(HaveOccurred())
			return cfg
		}

		It("accepts jwt mode with trusted issuers", func() {
			cfg := readConfig(`
security:
  mode: jwt
  trustedIssuers:
    - https://idp.example.com/realms/controlplane
`)
			Expect(cfg.Security.Mode).To(Equal(security.ModeJWT))
			Expect(cfg.Security.TrustedIssuers).To(ConsistOf("https://idp.example.com/realms/controlplane"))
		})

		It("accepts mock mode", func() {
			cfg := readConfig(`
security:
  mode: mock
`)
			Expect(cfg.Security.Mode).To(Equal(security.ModeMock))
		})

		It("preserves jwt default when no security section is present", func() {
			cfg := readConfig(`
database:
  url: postgres://localhost/test
`)
			Expect(cfg.Security.Mode).To(Equal(security.ModeJWT))
		})
	})

	Describe("Validate", func() {
		It("returns nil for jwt mode with trusted issuers", func() {
			sec := config.SecurityConfig{
				Mode:           "jwt",
				TrustedIssuers: []string{"https://idp.example.com/realms/controlplane"},
			}
			Expect(sec.Validate()).To(Succeed())
		})

		It("returns an error for jwt mode with no trusted issuers", func() {
			sec := config.SecurityConfig{
				Mode: "jwt",
			}
			Expect(sec.Validate()).To(MatchError(ContainSubstring("trustedIssuer")))
		})

		It("returns nil for mock mode", func() {
			sec := config.SecurityConfig{Mode: "mock"}
			Expect(sec.Validate()).To(Succeed())
		})

		It("returns an error for an unknown mode", func() {
			sec := config.SecurityConfig{Mode: "legacy"}
			Expect(sec.Validate()).To(MatchError(ContainSubstring("invalid security.mode")))
		})
	})
})
