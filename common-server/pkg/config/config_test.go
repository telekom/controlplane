// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/telekom/controlplane/common-server/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type logCfg struct {
	Level string `mapstructure:"level"`
}

type item struct {
	Name string `mapstructure:"name"`
}

type nestedCfg struct {
	Mode    string   `mapstructure:"mode"`
	Issuers []string `mapstructure:"trustedIssuers"`
}

type testCfg struct {
	Log     logCfg        `mapstructure:"log"`
	Timeout time.Duration `mapstructure:"timeout"`
	Items   []item        `mapstructure:"items"`
	Nested  nestedCfg     `mapstructure:"nested"`
}

func writeTempYAML(content string) string {
	dir := GinkgoT().TempDir()
	path := filepath.Join(dir, "config.yaml")
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	return path
}

var _ = Describe("Load", func() {
	It("returns defaults unchanged with empty path and no env", func() {
		defaults := &testCfg{Log: logCfg{Level: "info"}, Timeout: 30 * time.Second}
		cfg, err := config.Load("", defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Log.Level).To(Equal("info"))
		Expect(cfg.Timeout).To(Equal(30 * time.Second))
	})

	It("lets file values override defaults", func() {
		path := writeTempYAML("log:\n  level: debug\n")
		defaults := &testCfg{Log: logCfg{Level: "info"}}
		cfg, err := config.Load(path, defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Log.Level).To(Equal("debug"))
	})

	It("lets env override file and defaults", func() {
		path := writeTempYAML("log:\n  level: debug\n")
		GinkgoT().Setenv("LOG_LEVEL", "error")
		defaults := &testCfg{Log: logCfg{Level: "info"}}
		cfg, err := config.Load(path, defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Log.Level).To(Equal("error"))
	})

	It("returns an error for a missing file with non-empty path", func() {
		_, err := config.Load("/does/not/exist.yaml", &testCfg{})
		Expect(err).To(HaveOccurred())
		Expect(func() { config.LoadOrDie("/does/not/exist.yaml", &testCfg{}) }).To(Panic())
	})

	It("decodes a time.Duration from a string", func() {
		path := writeTempYAML("timeout: 55s\n")
		cfg, err := config.Load(path, &testCfg{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Timeout).To(Equal(55 * time.Second))
	})

	It("loads slice elements from a file", func() {
		path := writeTempYAML("items:\n  - name: a\n  - name: b\n")
		cfg, err := config.Load(path, &testCfg{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Items).To(HaveLen(2))
		Expect(cfg.Items[0].Name).To(Equal("a"))
		Expect(cfg.Items[1].Name).To(Equal("b"))
	})

	It("decodes an embedded BaseConfig (squash) alongside a server-specific field", func() {
		path := writeTempYAML("log:\n  level: debug\naddress: :9090\n")
		cfg, err := config.Load(path, &squashCfg{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Log.Level).To(Equal("debug"))
		Expect(cfg.Address).To(Equal(":9090"))
	})

	It("lets env override a nested scalar seeded from defaults", func() {
		GinkgoT().Setenv("NESTED_MODE", "mock")
		defaults := &testCfg{Nested: nestedCfg{Mode: "jwt"}}
		cfg, err := config.Load("", defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Nested.Mode).To(Equal("mock"))
	})

	It("lets env override a nested scalar under a squashed BaseConfig", func() {
		// Env must map from the squashed path (no "baseconfig" prefix): the key
		// is listeners... not baseconfig.listeners...
		GinkgoT().Setenv("LOG_LEVEL", "warn")
		defaults := &squashCfg{}
		defaults.Log.Level = "info"
		cfg, err := config.Load("", defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Log.Level).To(Equal("warn"))
	})

	It("splits a comma-separated env value into a string slice", func() {
		GinkgoT().Setenv("NESTED_TRUSTEDISSUERS", "https://a.example.com,https://b.example.com")
		defaults := &testCfg{Nested: nestedCfg{Issuers: []string{}}}
		cfg, err := config.Load("", defaults)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Nested.Issuers).To(Equal([]string{"https://a.example.com", "https://b.example.com"}))
	})
})

type squashCfg struct {
	config.BaseConfig `mapstructure:",squash"`
	Address           string `mapstructure:"address"`
}
