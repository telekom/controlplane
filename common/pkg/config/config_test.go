// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type sampleSpec struct {
	Feature string `yaml:"feature"`
	Count   int    `yaml:"count"`
}

func defaulter() sampleSpec {
	return sampleSpec{
		Count:   0,
		Feature: "test",
	}
}

// fixturePath returns the path to a static YAML configuration under testdata/.
func fixturePath(name string) string {
	return filepath.Join("testdata", name)
}

var _ = Describe("Controller config loading", func() {
	ctx := context.Background()
	Context("LoadFromFile -> EmptySpec", func() {
		It("merges defaults when values are missing", func() {
			cfg, err := LoadFromFile[EmptySpec](ctx, fixturePath("controller-defaults.yaml"), EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Common.Metrics.BindAddress).To(Equal(":9090"))
			Expect(cfg.Common.Metrics.SecureServing).To(BeTrue())
			Expect(cfg.Common.Probe.BindAddress).To(Equal(":7071"))
			Expect(cfg.Common.Log.Development).To(BeTrue())
			Expect(cfg.Common.EnableHTTP2).To(BeFalse())
			Expect(cfg.Common.Reconciler.RequeueAfterOnError).To(Equal(1 * time.Second))
			Expect(cfg.Common.Reconciler.RequeueAfter).To(Equal(30 * time.Minute))
			Expect(cfg.Common.Reconciler.JitterFactor).To(Equal(0.7))
			Expect(cfg.Common.Reconciler.MaxBackoff).To(Equal(5 * time.Minute))
			Expect(cfg.Common.Reconciler.MaxConcurrentReconciles).To(Equal(10))
		})

		It("loads reconciler overrides and derives the finalizer name", func() {
			cfg, err := LoadFromFile[EmptySpec](ctx, fixturePath("controller-reconciler-overrides.yaml"), EmptySpecDefault)
			Expect(err).ToNot(HaveOccurred())

			Expect(cfg.Common.Reconciler.RequeueAfterOnError).To(Equal(5 * time.Second))
			Expect(cfg.Common.Reconciler.RequeueAfter).To(Equal(1 * time.Minute))
			Expect(cfg.Common.Reconciler.JitterFactor).To(BeNumerically("==", 0.4))
			Expect(cfg.Common.Reconciler.MaxBackoff).To(Equal(2 * time.Minute))
			Expect(cfg.Common.Reconciler.MaxConcurrentReconciles).To(Equal(3))
		})
	})

	Context("LoadFromFile -> sampleSpec", func() {
		It("loads the generic spec alongside common config", func() {
			cfg, err := LoadFromFile[sampleSpec](ctx, fixturePath("app-with-spec.yaml"), defaulter)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Spec.Feature).To(Equal("genetics"))
			Expect(cfg.Spec.Count).To(Equal(42))
			Expect(cfg.Common.Metrics.BindAddress).To(Equal(":8181"))
			Expect(cfg.Common.Metrics.SecureServing).To(BeFalse())
			Expect(cfg.Common.Probe.BindAddress).To(Equal(":9191"))
			Expect(cfg.Common.EnableHTTP2).To(BeTrue())
			Expect(cfg.Common.Log.Development).To(BeFalse())
		})
	})
})
