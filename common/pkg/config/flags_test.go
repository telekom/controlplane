// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var _ = Describe("Config Flag Tests", func() {
	// Backup original values for restoration after tests
	var (
		origRequeueAfterOnError     time.Duration
		origRequeueAfter            time.Duration
		origDefaultNamespace        string
		origDefaultEnvironment      string
		origLabelKeyPrefix          string
		origFinalizerSuffix         string
		origJitterFactor            float64
		origMaxBackoff              time.Duration
		origMaxConcurrentReconciles int
		origFinalizerName           string
	)

	BeforeEach(func() {
		origRequeueAfterOnError = RequeueAfterOnError
		origRequeueAfter = RequeueAfter
		origDefaultNamespace = DefaultNamespace
		origDefaultEnvironment = DefaultEnvironment
		origLabelKeyPrefix = LabelKeyPrefix
		origFinalizerSuffix = FinalizerSuffix
		origJitterFactor = JitterFactor
		origMaxBackoff = MaxBackoff
		origMaxConcurrentReconciles = MaxConcurrentReconciles
		origFinalizerName = FinalizerName

		// Reset flag command line and viper for each test
		pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
		initViper()
		registerDefaults()
	})

	AfterEach(func() {
		// Restore original values
		RequeueAfterOnError = origRequeueAfterOnError
		RequeueAfter = origRequeueAfter
		DefaultNamespace = origDefaultNamespace
		DefaultEnvironment = origDefaultEnvironment
		LabelKeyPrefix = origLabelKeyPrefix
		FinalizerSuffix = origFinalizerSuffix
		JitterFactor = origJitterFactor
		MaxBackoff = origMaxBackoff
		MaxConcurrentReconciles = origMaxConcurrentReconciles
		FinalizerName = origFinalizerName
		viper.Reset()
	})

	initConfigFromFlags := func(args []string) {
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		os.Args = args

		Expect(registerFlag()).NotTo(HaveOccurred())
		loadConfig()
	}

	initConfigFromFile := func(configPath string) {
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		os.Args = []string{"program", "--config=" + configPath}
		Expect(registerFlag()).NotTo(HaveOccurred())
		Expect(registerConfigFileFromFlag()).NotTo(HaveOccurred())
		loadConfig()
	}

	initConfigFromEnv := func(envVars map[string]string) {
		oldEnv := map[string]string{}
		for key := range envVars {
			origVal, exists := os.LookupEnv(key)
			if exists {
				oldEnv[key] = origVal
			}
		}

		for key, value := range envVars {
			err := os.Setenv(key, value)
			Expect(err).NotTo(HaveOccurred())
		}

		defer func() {
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
		}()

		loadConfig()
	}

	Context("Command Line Flag Tests", func() {
		It("should apply string flag values", func() {
			initConfigFromFlags([]string{"program",
				"--default-namespace=test-namespace",
				"--default-environment=test-env",
				"--label-key-prefix=test.prefix",
				"--finalizer-suffix=test-finalizer"})

			Expect(DefaultNamespace).To(Equal("test-namespace"))
			Expect(DefaultEnvironment).To(Equal("test-env"))
			Expect(LabelKeyPrefix).To(Equal("test.prefix"))
			Expect(FinalizerSuffix).To(Equal("test-finalizer"))

			Expect(FinalizerName).To(Equal("test.prefix/test-finalizer"))
		})

		It("should apply numeric flag values", func() {
			initConfigFromFlags([]string{"program",
				"--requeue-after-on-error=5s",
				"--requeue-after=10m",
				"--jitter-factor=0.5",
				"--max-backoff=2m",
				"--max-concurrent-reconciles=15"})

			Expect(RequeueAfterOnError).To(Equal(5 * time.Second))
			Expect(RequeueAfter).To(Equal(10 * time.Minute))
			Expect(JitterFactor).To(Equal(0.5))
			Expect(MaxBackoff).To(Equal(2 * time.Minute))
			Expect(MaxConcurrentReconciles).To(Equal(15))
		})

		It("should use default values when no flags provided", func() {
			initConfigFromFlags([]string{"program"})
			Expect(RequeueAfterOnError).To(Equal(origRequeueAfterOnError))
			Expect(RequeueAfter).To(Equal(origRequeueAfter))
			Expect(DefaultNamespace).To(Equal(origDefaultNamespace))
		})

		It("should load config file path from flag", func() {
			initConfigFromFlags([]string{"program", "--config=testdata/config.yaml"})
			Expect(registerConfigFileFromFlag()).NotTo(HaveOccurred())
			Expect(viper.ConfigFileUsed()).To(Equal("testdata/config.yaml"))
		})
	})

	Context("Config File Tests", func() {

		It("should load values from config file", func() {
			configPath, err := filepath.Abs("testdata/config.yaml")
			Expect(err).NotTo(HaveOccurred())

			initConfigFromFile(configPath)

			Expect(RequeueAfterOnError).To(Equal(3 * time.Second))
			Expect(RequeueAfter).To(Equal(20 * time.Minute))
			Expect(DefaultNamespace).To(Equal("test-namespace-from-file"))
			Expect(DefaultEnvironment).To(Equal("test-env-from-file"))
			Expect(LabelKeyPrefix).To(Equal("test.prefix.from.file"))
			Expect(FinalizerSuffix).To(Equal("test-finalizer-from-file"))
			Expect(JitterFactor).To(Equal(0.5))
			Expect(MaxBackoff).To(Equal(3 * time.Minute))
			Expect(MaxConcurrentReconciles).To(Equal(5))
			Expect(FinalizerName).To(Equal("test.prefix.from.file/test-finalizer-from-file"))
		})

		It("should override default values from config file", func() {
			RequeueAfterOnError = 10 * time.Second
			DefaultNamespace = "original-namespace"
			configPath, err := filepath.Abs("testdata/config.yaml")
			Expect(err).NotTo(HaveOccurred())

			initConfigFromFile(configPath)
			Expect(RequeueAfterOnError).To(Equal(3 * time.Second))
			Expect(DefaultNamespace).To(Equal("test-namespace-from-file"))
		})
	})

	Context("Environment Variable Tests", func() {
		BeforeEach(func() {
			Expect(registerEnvs()).NotTo(HaveOccurred())
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
			initConfigFromEnv(envVars)

			// Verify environment variable values were applied
			Expect(RequeueAfterOnError).To(Equal(4 * time.Second))
			Expect(RequeueAfter).To(Equal(25 * time.Minute))
			Expect(DefaultNamespace).To(Equal("test-namespace-from-env"))
			Expect(DefaultEnvironment).To(Equal("test-env-from-env"))
			Expect(LabelKeyPrefix).To(Equal("test.prefix.from.env"))
			Expect(FinalizerSuffix).To(Equal("test-finalizer-from-env"))
			Expect(JitterFactor).To(Equal(0.6))
			Expect(MaxBackoff).To(Equal(4 * time.Minute))
			Expect(MaxConcurrentReconciles).To(Equal(7))

			// Verify derived values
			Expect(FinalizerName).To(Equal("test.prefix.from.env/test-finalizer-from-env"))
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
			initConfigFromEnv(envVars)

			// Verify environment variable values override default values
			Expect(RequeueAfterOnError).To(Equal(6 * time.Second))
			Expect(DefaultNamespace).To(Equal("override-namespace-from-env"))
		})
	})
})
