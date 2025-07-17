// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
)

var _ = Describe("Config Env Tests", func() {
	// Backup original values for restoration after tests
	var (
		origRequeueAfterOnError     time.Duration
		origRequeueAfter            time.Duration
		origDefaultNamespace        string
		origDefaultEnvironment      string
		origLabelKeyPrefix          string
		origJitterFactor            float64
		origMaxBackoff              time.Duration
		origMaxConcurrentReconciles int
		origFinalizerName           string

		oldEnv map[string]string
	)

	BeforeEach(func() {
		// Store original values
		origRequeueAfterOnError = RequeueAfterOnError
		origRequeueAfter = RequeueAfter
		origDefaultNamespace = DefaultNamespace
		origDefaultEnvironment = DefaultEnvironment
		origLabelKeyPrefix = LabelKeyPrefix
		origJitterFactor = JitterFactor
		origMaxBackoff = MaxBackoff
		origMaxConcurrentReconciles = MaxConcurrentReconciles
		origFinalizerName = FinalizerName

		// Reset viper for each test
		viper.Reset()
		registerDefaults()
		oldEnv = map[string]string{}
	})

	AfterEach(func() {
		// Restore original values
		RequeueAfterOnError = origRequeueAfterOnError
		RequeueAfter = origRequeueAfter
		DefaultNamespace = origDefaultNamespace
		DefaultEnvironment = origDefaultEnvironment
		LabelKeyPrefix = origLabelKeyPrefix
		JitterFactor = origJitterFactor
		MaxBackoff = origMaxBackoff
		MaxConcurrentReconciles = origMaxConcurrentReconciles
		FinalizerName = origFinalizerName
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
			// Expect(RequeueAfterOnError).To(Equal(origRequeueAfterOnError))
			Expect(RequeueAfter).To(Equal(origRequeueAfter))
			Expect(DefaultNamespace).To(Equal(origDefaultNamespace))
			Expect(DefaultEnvironment).To(Equal(origDefaultEnvironment))
			Expect(LabelKeyPrefix).To(Equal(origLabelKeyPrefix))
			Expect(JitterFactor).To(Equal(origJitterFactor))
			Expect(MaxBackoff).To(Equal(origMaxBackoff))
			Expect(MaxConcurrentReconciles).To(Equal(origMaxConcurrentReconciles))
			Expect(FinalizerName).To(Equal(origFinalizerName))
		})

		It("should load values from environment variables", func() {
			// Set up environment variables
			envVars := map[string]string{
				"REQUEUE_AFTER_ON_ERROR":    "4s",
				"REQUEUE_AFTER":             "25m",
				"DEFAULT_NAMESPACE":         "test-namespace-from-env",
				"DEFAULT_ENVIRONMENT":       "test-env-from-env",
				"LABEL_KEY_PREFIX":          "test.prefix.from.env",
				"FINALIZER_SUFFIX":          "test-finalizer-from-env",
				"JITTER_FACTOR":             "0.6",
				"MAX_BACKOFF":               "4m",
				"MAX_CONCURRENT_RECONCILES": "7",
			}

			// Initialize with environment variables
			initEnv(envVars)

			// Verify environment variable values were applied
			Expect(RequeueAfterOnError).To(Equal(4 * time.Second))
			Expect(RequeueAfter).To(Equal(25 * time.Minute))
			Expect(DefaultNamespace).To(Equal("test-namespace-from-env"))
			Expect(DefaultEnvironment).To(Equal("test-env-from-env"))
			Expect(LabelKeyPrefix).To(Equal("test.prefix.from.env"))
			Expect(JitterFactor).To(Equal(0.6))
			Expect(MaxBackoff).To(Equal(4 * time.Minute))
			Expect(MaxConcurrentReconciles).To(Equal(7))

			// Verify derived values
			Expect(FinalizerName).To(Equal("test.prefix.from.env/finalizer"))
			cleanUpEnv(envVars)
		})

		It("should override default values with environment variables", func() {
			// Set default values different from environment variables
			RequeueAfterOnError = 10 * time.Second
			DefaultNamespace = "original-namespace"

			// Set up environment variables
			envVars := map[string]string{
				"REQUEUE_AFTER_ON_ERROR": "6s",
				"DEFAULT_NAMESPACE":      "override-namespace-from-env",
			}

			// Initialize with environment variables
			initEnv(envVars)

			// Verify environment variable values override default values
			Expect(RequeueAfterOnError).To(Equal(6 * time.Second))
			Expect(DefaultNamespace).To(Equal("override-namespace-from-env"))

			// Verify other defaults remain unchanged
			Expect(DefaultEnvironment).To(Equal(origDefaultEnvironment))
			Expect(LabelKeyPrefix).To(Equal(origLabelKeyPrefix))
			Expect(JitterFactor).To(Equal(origJitterFactor))
			Expect(MaxBackoff).To(Equal(origMaxBackoff))
			Expect(MaxConcurrentReconciles).To(Equal(origMaxConcurrentReconciles))
			Expect(FinalizerName).To(Equal(LabelKeyPrefix + "/" + FinalizerSuffix))
			cleanUpEnv(envVars)
		})
	})
})
