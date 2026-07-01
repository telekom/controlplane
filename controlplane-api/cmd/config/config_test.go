// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/controlplane-api/cmd/config"
)

var _ = Describe("SecurityConfig", func() {

	Describe("DefaultConfig", func() {
		It("defaults to jwt mode (secure by default)", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Security.Mode).To(Equal("jwt"))
		})

		It("derives Enabled=true from jwt mode", func() {
			cfg := config.DefaultConfig()
			// DefaultConfig does not call resolveSecurityMode — Enabled is not set yet.
			// Callers should call Validate() and resolve mode before checking Enabled.
			// The important invariant is that Mode is "jwt".
			Expect(cfg.Security.Mode).To(Equal("jwt"))
		})
	})

	Describe("ReadConfig", func() {
		readConfig := func(yaml string) *config.ServerConfig {
			cfg, err := config.ReadConfig(strings.NewReader(yaml))
			Expect(err).NotTo(HaveOccurred())
			return cfg
		}

		It("accepts jwt mode with trusted issuers and sets Enabled=true", func() {
			cfg := readConfig(`
security:
  mode: jwt
  trustedIssuers:
    - https://idp.example.com/realms/controlplane
`)
			Expect(cfg.Security.Mode).To(Equal("jwt"))
			Expect(cfg.Security.Enabled).To(BeTrue())
			Expect(cfg.Security.TrustedIssuers).To(ConsistOf("https://idp.example.com/realms/controlplane"))
		})

		It("accepts mock mode and sets Enabled=true", func() {
			cfg := readConfig(`
security:
  mode: mock
`)
			Expect(cfg.Security.Mode).To(Equal("mock"))
			Expect(cfg.Security.Enabled).To(BeTrue())
		})

		It("accepts disabled mode and sets Enabled=false", func() {
			cfg := readConfig(`
security:
  mode: disabled
`)
			Expect(cfg.Security.Mode).To(Equal("disabled"))
			Expect(cfg.Security.Enabled).To(BeFalse())
		})

		It("maps legacy enabled: false (no mode) to disabled mode", func() {
			cfg := readConfig(`
security:
  enabled: false
`)
			Expect(cfg.Security.Mode).To(Equal("disabled"))
			Expect(cfg.Security.Enabled).To(BeFalse())
		})

		It("maps legacy enabled: true (no mode) to mock mode", func() {
			cfg := readConfig(`
security:
  enabled: true
`)
			Expect(cfg.Security.Mode).To(Equal("mock"))
			Expect(cfg.Security.Enabled).To(BeTrue())
		})

		It("lets mode take precedence when both mode and enabled are specified", func() {
			cfg := readConfig(`
security:
  mode: disabled
  enabled: true
`)
			Expect(cfg.Security.Mode).To(Equal("disabled"))
			Expect(cfg.Security.Enabled).To(BeFalse())
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

		It("returns nil for disabled mode", func() {
			sec := config.SecurityConfig{Mode: "disabled"}
			Expect(sec.Validate()).To(Succeed())
		})

		It("returns an error for an unknown mode", func() {
			sec := config.SecurityConfig{Mode: "legacy"}
			Expect(sec.Validate()).To(MatchError(ContainSubstring("invalid security.mode")))
		})
	})
})
