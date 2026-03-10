// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Defaults", func() {
	Describe("defaultControllerConfig", func() {
		It("returns a valid default controller configuration with all required fields", func() {
			cfg := defaultControllerConfig()
			Expect(cfg).NotTo(BeNil())
			AssertAllDefaultsSet(&cfg)
			AssertBindAddressesSet(cfg.Metrics, cfg.Probe)
		})
	})

	Describe("defaultConfig", func() {
		It("creates config with EmptySpec", func() {
			// Test with EmptySpec
			cfg := defaultConfig[EmptySpec](EmptySpecDefault)
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Common).NotTo(BeNil())
			AssertAllDefaultsSet(&cfg.Common)
			Expect(cfg.Spec).To(Equal(EmptySpec{}))
		})

		It("creates config with custom spec types", func() {

			// Test with custom spec
			type customSpec struct {
				Value string
			}
			cfg := defaultConfig[customSpec](func() customSpec {
				return customSpec{Value: "test-value"}
			})
			Expect(cfg.Spec.Value).To(Equal("test-value"))
			Expect(cfg.Common.Metrics.BindAddress).To(Equal("0"))
			AssertAllDefaultsSet(&cfg.Common)

			// Test with different spec types
			type spec1 struct {
				Field1 string
			}
			type spec2 struct {
				Field2 int
			}
			cfg1 := defaultConfig[spec1](func() spec1 { return spec1{Field1: "value1"} })
			Expect(cfg1.Spec.Field1).To(Equal("value1"))
			AssertAllDefaultsSet(&cfg1.Common)

			cfg2 := defaultConfig[spec2](func() spec2 { return spec2{Field2: 42} })
			Expect(cfg2.Spec.Field2).To(Equal(42))
			AssertAllDefaultsSet(&cfg2.Common)

		})
	})
})

// Fixture paths
const (
	FixtureControllerDefaults = "controller-defaults.yaml"
	FixtureEmpty              = "empty.yaml"
)

// fixturePath returns the path to a static YAML configuration under testdata/.
func fixturePath(name string) string {
	return filepath.Join("testdata", name)
}

// Test spec types
type sampleSpec struct {
	Feature string `yaml:"feature"`
	Count   int    `yaml:"count"`
}

func defaultSampleSpec() sampleSpec {
	return sampleSpec{
		Count:   0,
		Feature: "test",
	}
}

// Assertion helpers for common validations

// AssertDefaultMetrics validates the default metrics configuration.
func AssertDefaultMetrics(metrics MetricsConfig) {
	Expect(metrics.BindAddress).To(Equal("0"))
	Expect(metrics.SecureServing).To(BeTrue())
	Expect(metrics.Cert.Name).To(Equal("tls.crt"))
	Expect(metrics.Cert.Key).To(Equal("tls.key"))
	Expect(metrics.Cert.Path).To(Equal(""))
}

// AssertDefaultProbe validates the default probe configuration.
func AssertDefaultProbe(probe ProbeConfig) {
	Expect(probe.BindAddress).To(Equal(":8081"))
}

// AssertDefaultReconciler validates the default reconciler configuration.
func AssertDefaultReconciler(reconciler ReconcilerConfig) {
	Expect(reconciler.RequeueAfterOnError).To(Equal(1 * time.Second))
	Expect(reconciler.RequeueAfter).To(Equal(30 * time.Minute))
	Expect(reconciler.JitterFactor).To(Equal(0.7))
	Expect(reconciler.MaxBackoff).To(Equal(5 * time.Minute))
	Expect(reconciler.MaxConcurrentReconciles).To(Equal(10))
}

// AssertDefaultLog validates the default log configuration.
func AssertDefaultLog(log LogConfig) {
	Expect(log.Development).To(BeTrue())
}

// AssertDefaultWebhook validates the default webhook configuration.
func AssertDefaultWebhook(webhook WebhookConfig) {
	Expect(webhook.Cert.Name).To(Equal("tls.crt"))
	Expect(webhook.Cert.Key).To(Equal("tls.key"))
	Expect(webhook.Cert.Path).To(Equal(""))
}

// AssertAllDefaultsSet validates that all required default fields are set.
func AssertAllDefaultsSet(cfg *ControllerConfig) {
	AssertDefaultMetrics(cfg.Metrics)
	AssertDefaultProbe(cfg.Probe)
	AssertDefaultReconciler(cfg.Reconciler)
	AssertDefaultLog(cfg.Log)
	AssertDefaultWebhook(cfg.Webhook)
	AssertDefaultFeatures(cfg.Features)
	Expect(cfg.EnableHTTP2).To(BeFalse())
}

// AssertBindAddressesSet validates that bind addresses are not empty.
func AssertBindAddressesSet(metrics MetricsConfig, probe ProbeConfig) {
	Expect(metrics.BindAddress).NotTo(BeEmpty())
	Expect(probe.BindAddress).NotTo(BeEmpty())
}

// AssertDefaultFeatures validates the default feature flags.
func AssertDefaultFeatures(features []FeatureConfig) {
	Expect(features).To(HaveLen(3))
	Expect(features[0].Name).To(Equal("pubsub"))
	Expect(features[0].Enabled).To(BeFalse())
	Expect(features[1].Name).To(Equal("secret_manager"))
	Expect(features[1].Enabled).To(BeTrue())
	Expect(features[2].Name).To(Equal("file_manager"))
	Expect(features[2].Enabled).To(BeTrue())
}
