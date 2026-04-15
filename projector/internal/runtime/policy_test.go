// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime_test

import (
	"time"

	"github.com/telekom/controlplane/projector/internal/config"
	"github.com/telekom/controlplane/projector/internal/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ErrorPolicy", func() {
	Describe("DefaultErrorPolicy", func() {
		var policy runtime.ErrorPolicy

		BeforeEach(func() {
			policy = runtime.DefaultErrorPolicy()
		})

		It("has a non-zero SkipRequeue", func() {
			Expect(policy.SkipRequeue).To(Equal(5 * time.Minute))
		})

		It("has a non-zero DependencyDelay", func() {
			Expect(policy.DependencyDelay).To(Equal(1 * time.Second))
		})

		It("has zero DependencyJitter (backward compatible)", func() {
			Expect(policy.DependencyJitter).To(Equal(time.Duration(0)))
		})

		It("has a non-zero PeriodicResync", func() {
			Expect(policy.PeriodicResync).To(Equal(5 * time.Minute))
		})

		It("all non-jitter durations are positive", func() {
			Expect(policy.SkipRequeue).To(BeNumerically(">", 0))
			Expect(policy.DependencyDelay).To(BeNumerically(">", 0))
			Expect(policy.PeriodicResync).To(BeNumerically(">", 0))
		})
	})

	Describe("NewErrorPolicyFromConfig", func() {
		It("maps all config fields to policy fields", func() {
			cfg := &config.Config{
				SkipRequeue:           10 * time.Minute,
				DependencyDelay:       3 * time.Second,
				DependencyDelayJitter: 5 * time.Second,
				PeriodicResync:        30 * time.Second,
			}

			policy := runtime.NewErrorPolicyFromConfig(cfg)

			Expect(policy.SkipRequeue).To(Equal(10 * time.Minute))
			Expect(policy.DependencyDelay).To(Equal(3 * time.Second))
			Expect(policy.DependencyJitter).To(Equal(5 * time.Second))
			Expect(policy.PeriodicResync).To(Equal(30 * time.Second))
		})

		It("preserves zero PeriodicResync (event-driven)", func() {
			cfg := &config.Config{
				SkipRequeue:           5 * time.Minute,
				DependencyDelay:       2 * time.Second,
				DependencyDelayJitter: 3 * time.Second,
				PeriodicResync:        0,
			}

			policy := runtime.NewErrorPolicyFromConfig(cfg)

			Expect(policy.PeriodicResync).To(Equal(time.Duration(0)))
		})

		It("preserves zero DependencyJitter (no jitter)", func() {
			cfg := &config.Config{
				SkipRequeue:           5 * time.Minute,
				DependencyDelay:       2 * time.Second,
				DependencyDelayJitter: 0,
				PeriodicResync:        0,
			}

			policy := runtime.NewErrorPolicyFromConfig(cfg)

			Expect(policy.DependencyJitter).To(Equal(time.Duration(0)))
		})
	})
})
