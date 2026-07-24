// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/controlplane-api/cmd/config"
)

var _ = Describe("SecurityConfig", func() {

	writeConfig := func(yaml string) string {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "config.yaml")
		Expect(os.WriteFile(path, []byte(yaml), 0o600)).To(Succeed())
		return path
	}

	Describe("DefaultConfig", func() {
		It("defaults to an external jwt listener (secure by default)", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Listeners.External).NotTo(BeNil())
			Expect(cfg.Listeners.External.JWT).NotTo(BeNil())
			Expect(cfg.Listeners.External.JWT.Mode).To(Equal(security.ModeJWT))
		})

		It("has no internal listener (external-only)", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Listeners.Internal).To(BeNil())
		})

		It("defaults TLS to the standard cert/key paths", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.TLS).NotTo(BeNil())
			Expect(cfg.TLS.Cert).To(Equal("/etc/tls/tls.crt"))
			Expect(cfg.TLS.Key).To(Equal("/etc/tls/tls.key"))
		})
	})

	Describe("GetConfigOrDie", func() {
		It("accepts jwt mode with trusted issuers", func() {
			path := writeConfig(`
listeners:
  external:
    address: ":8443"
    jwt:
      mode: jwt
      trustedIssuers:
        - https://idp.example.com/realms/controlplane
`)
			cfg := config.GetConfigOrDie(path)
			Expect(cfg.Listeners.External.JWT.Mode).To(Equal(security.ModeJWT))
			Expect(cfg.Listeners.External.JWT.TrustedIssuers).To(ConsistOf("https://idp.example.com/realms/controlplane"))
		})

		It("accepts mock mode", func() {
			path := writeConfig(`
listeners:
  external:
    address: ":8443"
    jwt:
      mode: mock
`)
			cfg := config.GetConfigOrDie(path)
			Expect(cfg.Listeners.External.JWT.Mode).To(Equal(security.ModeMock))
		})

		It("fails closed when jwt mode is configured without trusted issuers", func() {
			path := writeConfig(`
database:
  url: postgres://localhost/test
`)
			// The default external listener is jwt mode; jwt without issuers is
			// rejected fail-closed rather than starting fail-open.
			Expect(func() { config.GetConfigOrDie(path) }).To(Panic())
		})
	})
})
