// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

const baseConfig = `
storePath: /tmp/store
sources:
  default:
    environment: env
    zone: zone
    url: https://gateway.example.com/admin
    tokenUrl: https://idp.example.com/auth/realms/rover
    clientId: client
    clientSecret: secret
`

var _ = Describe("LoadConfig", func() {
	load := func(content string) *config.Config {
		path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
		cfg, err := config.LoadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		return &cfg
	}

	It("verifies TLS by default", func() {
		source, ok := load(baseConfig).GetSourceConfig("default")
		Expect(ok).To(BeTrue())
		Expect(source.InsecureSkipVerify).To(BeFalse())
	})

	It("reads insecureSkipVerify", func() {
		source, ok := load(baseConfig + "    insecureSkipVerify: true\n").GetSourceConfig("default")
		Expect(ok).To(BeTrue())
		Expect(source.InsecureSkipVerify).To(BeTrue())
	})
})
