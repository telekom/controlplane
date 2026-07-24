// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/discovery-server/internal/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

// writeTempConfig writes a minimal valid deployment config (issuers are only
// known at deploy time) and returns its path.
func writeTempConfig(yaml string) string {
	f, err := os.CreateTemp("", "discovery-config-*.yaml")
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
		path := writeTempConfig(`
listeners:
  external:
    jwt:
      trustedIssuers:
        - https://issuer.example
`)
		cfg := config.LoadConfig(path)

		Expect(cfg.Informer.DisableCache).To(BeTrue())
		Expect(cfg.Log.Level).To(Equal("info"))
		Expect(cfg.Log.Encoding).To(Equal("json"))
		Expect(cfg.Database.Filepath).To(Equal(""))
		Expect(cfg.Database.ReduceMemory).To(BeFalse())
	})
})
