// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"time"

	"github.com/spf13/viper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config Env Tests", func() {
	// Backup original values for restoration after tests
	var (
		origRequeueAfterOnError     time.Duration
		origRequeueAfter            time.Duration
		origJitterFactor            float64
		origMaxBackoff              time.Duration
		origMaxConcurrentReconciles int

		oldEnv map[string]string
	)

	BeforeEach(func() {
		// Store original values
		origRequeueAfterOnError = RequeueAfterOnError
		origRequeueAfter = RequeueAfter
		origJitterFactor = JitterFactor
		origMaxBackoff = MaxBackoff
		origMaxConcurrentReconciles = MaxConcurrentReconciles

		// Reset viper for each test
		viper.Reset()
		registerDefaults()
		oldEnv = map[string]string{}
	})

	AfterEach(func() {
		// Restore original values
		RequeueAfterOnError = origRequeueAfterOnError
		RequeueAfter = origRequeueAfter
		JitterFactor = origJitterFactor
		MaxBackoff = origMaxBackoff
		MaxConcurrentReconciles = origMaxConcurrentReconciles
		viper.Reset()
	})

	initEnv := func(envVars map[string]string) {
		// Clear oldEnv map first to prevent re-using old values
		oldEnv = map[string]string{}

		// Save current environment variables
		for key := range envVars {
			origVal, exists := os.LookupEnv(key)
			if exists {
				oldEnv[key] = origVal
			}
		}

		// Set new environment variables
		for key, value := range envVars {
			err := os.Setenv(key, value)
			Expect(err).NotTo(HaveOccurred())
		}

		registerEnvs()
		Parse()
	}

	cleanUpEnv := func(envVars map[string]string) {
		for key := range envVars {
			// If it existed before, restore it
			if origVal, exists := oldEnv[key]; exists {
				err := os.Setenv(key, origVal)
				Expect(err).NotTo(HaveOccurred())
			} else {
				// Otherwise, unset it
				err := os.Unsetenv(key)
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}

	Context("Default Values with Explicit Registration", func() {
		It("should use default values when env not provided", func() {
			// Ensure flags are registered but no values passed
			registerEnvs()
			Parse()

			// Verify all values match defaults
			Expect(RequeueAfterOnError).To(Equal(origRequeueAfterOnError))
			Expect(RequeueAfter).To(Equal(origRequeueAfter))
			Expect(JitterFactor).To(Equal(origJitterFactor))
			Expect(MaxBackoff).To(Equal(origMaxBackoff))
			Expect(MaxConcurrentReconciles).To(Equal(origMaxConcurrentReconciles))
		})

		It("should load values from environment variables", func() {
			// Set up environment variables
			envVars := map[string]string{
				"REQUEUE_AFTER_ON_ERROR":    "4s",
				"REQUEUE_AFTER":             "25m",
				"JITTER_FACTOR":             "0.6",
				"MAX_BACKOFF":               "4m",
				"MAX_CONCURRENT_RECONCILES": "7",
			}

			// Initialize with environment variables
			initEnv(envVars)

			// Verify environment variable values were applied
			Expect(RequeueAfterOnError).To(Equal(4 * time.Second))
			Expect(RequeueAfter).To(Equal(25 * time.Minute))
			Expect(JitterFactor).To(Equal(0.6))
			Expect(MaxBackoff).To(Equal(4 * time.Minute))
			Expect(MaxConcurrentReconciles).To(Equal(7))

			cleanUpEnv(envVars)
		})

		It("should ignore label prefix overrides from environment variables and Viper", func() {
			envVars := map[string]string{
				"LABEL_KEY_PREFIX": "test.prefix.from.env",
			}
			DeferCleanup(func() { cleanUpEnv(envVars) })

			initEnv(envVars)
			Expect(BuildLabelKey("environment")).To(Equal("cp.ei.telekom.de/environment"))
			Expect(FinalizerName).To(Equal("cp.ei.telekom.de/finalizer"))

			viper.Set("label-key-prefix", "test.prefix.from.viper")
			Parse()
			Expect(BuildLabelKey("environment")).To(Equal("cp.ei.telekom.de/environment"))
			Expect(FinalizerName).To(Equal("cp.ei.telekom.de/finalizer"))
		})

		It("should ignore namespace and environment overrides from environment variables and Viper", func() {
			envVars := map[string]string{
				"DEFAULT_NAMESPACE":   "test-namespace-from-env",
				"DEFAULT_ENVIRONMENT": "test-env-from-env",
			}
			DeferCleanup(func() { cleanUpEnv(envVars) })

			initEnv(envVars)
			Expect(DefaultNamespace).To(Equal("default"))
			Expect(DefaultEnvironment).To(Equal("default"))

			viper.Set("default-namespace", "test-namespace-from-viper")
			viper.Set("default-environment", "test-env-from-viper")
			Parse()
			Expect(DefaultNamespace).To(Equal("default"))
			Expect(DefaultEnvironment).To(Equal("default"))
		})

		It("should override default values with environment variables", func() {
			// Set default values different from environment variables
			RequeueAfterOnError = 10 * time.Second

			// Set up environment variables
			envVars := map[string]string{
				"REQUEUE_AFTER_ON_ERROR": "6s",
			}

			// Initialize with environment variables
			initEnv(envVars)

			// Verify environment variable values override default values
			Expect(RequeueAfterOnError).To(Equal(6 * time.Second))

			// Verify other defaults remain unchanged
			Expect(JitterFactor).To(Equal(origJitterFactor))
			Expect(MaxBackoff).To(Equal(origMaxBackoff))
			Expect(MaxConcurrentReconciles).To(Equal(origMaxConcurrentReconciles))
			cleanUpEnv(envVars)
		})
	})
})
