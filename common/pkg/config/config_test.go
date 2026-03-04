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

type sampleSpec struct {
	Feature string `yaml:"feature"`
	Count   int    `yaml:"count"`
}

// fixturePath returns the path to a static YAML configuration under testdata/.
func fixturePath(name string) string {
	return filepath.Join("testdata", name)
}

var _ = Describe("Controller config loading", func() {
	Context("LoadFromFile -> EmptySpec", func() {
		It("merges defaults when values are missing", func() {
			cfg, err := LoadFromFile[EmptySpec](fixturePath("controller-defaults.yaml"))
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Common.Metrics.BindAddress).To(Equal(":9090"))
			Expect(cfg.Common.Metrics.SecureServing).To(BeTrue())
			Expect(cfg.Common.Probe.BindAddress).To(Equal(":7071"))
			Expect(cfg.Common.LeaderElection.Enabled).To(BeTrue())
			Expect(cfg.Common.LeaderElection.ID).To(Equal("abcdef-default-controller"))
			Expect(cfg.Common.Log.Development).To(BeTrue())
			Expect(cfg.Common.EnableHTTP2).To(BeFalse())
			Expect(cfg.Common.Reconciler.RequeueAfterOnError).To(Equal(1 * time.Second))
			Expect(cfg.Common.Reconciler.RequeueAfter).To(Equal(30 * time.Minute))
			Expect(cfg.Common.Reconciler.JitterFactor).To(Equal(0.7))
			Expect(cfg.Common.Reconciler.MaxBackoff).To(Equal(5 * time.Minute))
			Expect(cfg.Common.Reconciler.MaxConcurrentReconciles).To(Equal(10))
			Expect(cfg.Common.Reconciler.DefaultNamespace).To(Equal("default"))
			Expect(cfg.Common.Reconciler.DefaultEnvironment).To(Equal("default"))
			Expect(cfg.Common.Reconciler.LabelKeyPrefix).To(Equal("cp.ei.telekom.de"))
			Expect(cfg.Common.Reconciler.FinalizerName).To(Equal("cp.ei.telekom.de/finalizer"))
		})

		It("returns an error when required fields are missing", func() {
			_, err := LoadFromFile[EmptySpec](fixturePath("controller-leader-election-invalid.yaml"))
			Expect(err).To(HaveOccurred())
		})

		It("loads reconciler overrides and derives the finalizer name", func() {
			cfg, err := LoadFromFile[EmptySpec](fixturePath("controller-reconciler-overrides.yaml"))
			Expect(err).ToNot(HaveOccurred())

			Expect(cfg.Common.Reconciler.RequeueAfterOnError).To(Equal(5 * time.Second))
			Expect(cfg.Common.Reconciler.RequeueAfter).To(Equal(1 * time.Minute))
			Expect(cfg.Common.Reconciler.JitterFactor).To(BeNumerically("==", 0.4))
			Expect(cfg.Common.Reconciler.MaxBackoff).To(Equal(2 * time.Minute))
			Expect(cfg.Common.Reconciler.MaxConcurrentReconciles).To(Equal(3))
			Expect(cfg.Common.Reconciler.DefaultNamespace).To(Equal("custom-ns"))
			Expect(cfg.Common.Reconciler.DefaultEnvironment).To(Equal("prod"))
			Expect(cfg.Common.Reconciler.LabelKeyPrefix).To(Equal("custom.prefix"))
			Expect(cfg.Common.Reconciler.FinalizerName).To(Equal("custom.prefix/finalizer"))
		})
	})

	Context("LoadFromFile -> sampleSpec", func() {
		It("loads the generic spec alongside common config", func() {
			cfg, err := LoadFromFile[sampleSpec](fixturePath("app-with-spec.yaml"))
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Spec.Feature).To(Equal("genetics"))
			Expect(cfg.Spec.Count).To(Equal(42))
			Expect(cfg.Common.Metrics.BindAddress).To(Equal(":8181"))
			Expect(cfg.Common.Metrics.SecureServing).To(BeFalse())
			Expect(cfg.Common.Probe.BindAddress).To(Equal(":9191"))
			Expect(cfg.Common.LeaderElection.Enabled).To(BeTrue())
			Expect(cfg.Common.LeaderElection.ID).To(Equal("sample-controller"))
			Expect(cfg.Common.EnableHTTP2).To(BeTrue())
			Expect(cfg.Common.Log.Development).To(BeFalse())
		})
	})
})
