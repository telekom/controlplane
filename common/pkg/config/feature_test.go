// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"maps"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
)

var _ = Describe("Feature Tests", func() {
	var (
		origFeatures map[Feature]bool
		oldEnv       map[string]string
	)

	BeforeEach(func() {
		// Save feature state
		origFeatures = make(map[Feature]bool)
		for f, enabled := range features {
			origFeatures[f] = enabled
		}

		// Reset viper for each test
		viper.Reset()
		registerDefaults()
		oldEnv = map[string]string{}
	})

	AfterEach(func() {
		// Restore feature state
		features = make(map[Feature]bool)
		maps.Copy(features, origFeatures)
		viper.Reset()
	})

	initEnv := func(envVars map[string]string) {
		oldEnv = map[string]string{}
		for key := range envVars {
			if origVal, exists := os.LookupEnv(key); exists {
				oldEnv[key] = origVal
			}
		}
		for key, value := range envVars {
			Expect(os.Setenv(key, value)).To(Succeed())
		}
		registerEnvs()
		Parse()
	}

	cleanUpEnv := func(envVars map[string]string) {
		for key := range envVars {
			if origVal, exists := oldEnv[key]; exists {
				Expect(os.Setenv(key, origVal)).To(Succeed())
			} else {
				Expect(os.Unsetenv(key)).To(Succeed())
			}
		}
	}

	Context("Feature interface", func() {
		It("String() should return the feature name", func() {
			Expect(FeaturePubSub.String()).To(Equal("pubsub"))
			Expect(FeatureSecretManager.String()).To(Equal("secret_manager"))
			Expect(FeatureFileManager.String()).To(Equal("file_manager"))
		})

		It("Path() should return the Viper config path", func() {
			Expect(FeaturePubSub.Path()).To(Equal("feature-pubsub-enabled"))
			Expect(FeatureSecretManager.Path()).To(Equal("feature-secret_manager-enabled"))
			Expect(FeatureFileManager.Path()).To(Equal("feature-file_manager-enabled"))
		})
	})

	Context("Default feature states", func() {
		It("FeaturePubSub should be disabled by default", func() {
			Expect(FeaturePubSub.IsEnabled()).To(BeFalse())
		})

		It("FeatureSecretManager should be enabled by default", func() {
			Expect(FeatureSecretManager.IsEnabled()).To(BeTrue())
		})

		It("FeatureFileManager should be enabled by default", func() {
			Expect(FeatureFileManager.IsEnabled()).To(BeTrue())
		})
	})

	Context("Feature override via environment variables", func() {
		It("should enable a disabled feature when env var is set to true", func() {
			envVars := map[string]string{
				"FEATURE_PUBSUB_ENABLED": "true",
			}
			initEnv(envVars)

			Expect(FeaturePubSub.IsEnabled()).To(BeTrue())
			cleanUpEnv(envVars)
		})

		It("should disable an enabled feature when env var is set to false", func() {
			envVars := map[string]string{
				"FEATURE_SECRET_MANAGER_ENABLED": "false",
			}
			initEnv(envVars)

			Expect(FeatureSecretManager.IsEnabled()).To(BeFalse())
			cleanUpEnv(envVars)
		})
	})

	Context("NewFeature", func() {
		var testFeature Feature

		AfterEach(func() {
			// Clean up the test feature from the map
			if testFeature != nil {
				delete(features, testFeature)
			}
		})

		It("should register the feature in the features map", func() {
			testFeature = NewFeature("test_feature", false)

			Expect(features).To(HaveKey(testFeature))
			Expect(features[testFeature]).To(BeFalse())
		})

		It("should set the Viper default for the feature", func() {
			testFeature = NewFeature("test_feature_default", true)

			Expect(viper.GetBool(testFeature.Path())).To(BeTrue())
		})
	})
})
