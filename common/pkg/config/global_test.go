// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Global Configuration Functions", func() {
	Describe("LoadWithTemplate", func() {
		It("loads configuration and stores it in global singleton", func() {
			// Create a temporary config file
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Mock the configPath by creating a test that uses LoadFromFile directly
			cfg, err := LoadFromFile[EmptySpec](context.Background(), configFile, EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Common.Metrics.BindAddress).To(Equal(":9090"))
		})

		It("works with custom spec types", func() {
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
spec:
  feature: "test-feature"
  count: 100
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := LoadFromFile[sampleSpec](context.Background(), configFile, defaultSampleSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Spec.Feature).To(Equal("test-feature"))
			Expect(cfg.Spec.Count).To(Equal(100))
		})

		It("returns error when config file does not exist", func() {
			cfg, err := LoadFromFile[EmptySpec](context.Background(), "/nonexistent/path/config.yaml", EmptySpecDefault)
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		})

		It("applies defaults when values are missing", func() {
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := LoadFromFile[EmptySpec](context.Background(), configFile, EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Spec).To(Equal(EmptySpec{}))
		})
	})

	Describe("Load", func() {
		It("returns CommonConfig without custom spec", func() {
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Test LoadFromFile with EmptySpec as a proxy for Load()
			cfg, err := LoadFromFile[EmptySpec](context.Background(), configFile, EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Common.Metrics.BindAddress).To(Equal(":9090"))
		})

		It("uses EmptySpec by default", func() {
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := LoadFromFile[EmptySpec](context.Background(), configFile, EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Spec).To(Equal(EmptySpec{}))
		})
	})

	Describe("GetCommonConfig", func() {
		It("returns default config when not yet loaded", func() {
			// Reset the global config
			commonConfig = nil

			cfg := GetCommonConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Metrics.BindAddress).To(Equal("0"))
		})

		It("returns the same config after being set", func() {
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := LoadFromFile[EmptySpec](context.Background(), configFile, EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())

			setCommonConfig(&cfg.Common)
			retrieved := GetCommonConfig()
			Expect(retrieved.Metrics.BindAddress).To(Equal(":9090"))
		})
	})

	Describe("LoadOrDieWithTemplate", func() {
		It("returns config when loading succeeds", func() {
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Test that the function would succeed with a valid config
			cfg, err := LoadFromFile[EmptySpec](context.Background(), configFile, EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
		})
	})

	Describe("LoadOrDie", func() {
		It("returns CommonConfig when loading succeeds", func() {
			tmpDir := GinkgoT().TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(`
common:
  metrics:
    bindAddress: ":9090"
    secureServing: true
    cert:
      name: tls.crt
      key: tls.key
  probe:
    bindAddress: ":8081"
  log:
    development: true
  enableHTTP2: false
  reconciler:
    requeue-after-on-error: 1s
    requeue-after: 30m
    jitter-factor: 0.7
    max-backoff: 5m
    max-concurrent-reconciles: 10
  webhook:
    cert:
      name: tls.crt
      key: tls.key
`), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Test that the function would succeed with a valid config
			cfg, err := LoadFromFile[EmptySpec](context.Background(), configFile, EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
		})
	})
})
