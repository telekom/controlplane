// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"testing"

	"github.com/telekom/controlplane/organization-server/internal/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

// writeTempConfig writes a config file (issuers are only known at deploy time)
// and returns its path.
func writeTempConfig(yaml string) string {
	f, err := os.CreateTemp("", "organization-config-*.yaml")
	Expect(err).NotTo(HaveOccurred())
	_, err = f.WriteString(yaml)
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Close()).To(Succeed())
	DeferCleanup(func() { _ = os.Remove(f.Name()) })
	return f.Name()
}

var _ = Describe("LoadConfig", func() {
	It("fails without deployment-provided jwt issuers", func() {
		Expect(func() { config.LoadConfig("") }).To(Panic())
	})

	It("returns typed defaults once issuers are provided", func() {
		cfg := config.LoadConfig(writeTempConfig(`
listeners:
  external:
    jwt:
      trustedIssuers:
        - https://issuer.example
`))

		Expect(cfg.Log.Level).To(Equal("info"))
		Expect(cfg.Log.Encoding).To(Equal("json"))
		Expect(cfg.Listeners.External.Address).To(Equal(":8443"))
		Expect(cfg.Listeners.Internal).To(BeNil())
		Expect(cfg.TLS.Cert).To(Equal("/etc/tls/tls.crt"))
		Expect(cfg.Rover.Environment).To(Equal("controlplane"))
		Expect(cfg.Rover.TokenFilePath).To(Equal("/var/run/secrets/rover/token"))
		Expect(cfg.Rover.CaFilePath).To(Equal("/var/run/secrets/trust-bundle/trust-bundle.pem"))
		Expect(cfg.CPAPI.TokenFilePath).To(Equal("/var/run/secrets/cpapi/token"))
		Expect(cfg.CPAPI.CaFilePath).To(Equal("/var/run/secrets/trust-bundle/trust-bundle.pem"))
	})

	It("overrides from the environment", func() {
		GinkgoT().Setenv("ROVER_ENDPOINT", "http://custom-rover")
		GinkgoT().Setenv("LISTENERS_EXTERNAL_JWT_TRUSTEDISSUERS", "https://a.example,https://b.example")

		cfg := config.LoadConfig("")

		Expect(cfg.Rover.Endpoint).To(Equal("http://custom-rover"))
		Expect(cfg.Listeners.External.JWT.TrustedIssuers).To(Equal([]string{"https://a.example", "https://b.example"}))
	})
})
