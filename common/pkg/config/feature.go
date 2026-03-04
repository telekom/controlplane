// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"log/slog"

	"github.com/spf13/viper"
)

type Feature interface {
	String() string
	IsEnabled() bool
	Path() string
}

func NewFeature(name string, enabled bool) Feature {
	f := feature{name: name}
	features[f] = enabled
	viper.SetDefault(f.Path(), enabled)
	return f
}

type feature struct {
	name string
}

func (f feature) String() string {
	return f.name
}

func (f feature) IsEnabled() bool {
	return features[f]
}

func (f feature) Path() string {
	return "feature-" + f.name + "-enabled"
}

var (
	features                     = map[Feature]bool{}
	FeaturePubSub        Feature = NewFeature("pubsub", false)        // Pub/Sub feature disabled by default
	FeatureSecretManager Feature = NewFeature("secret_manager", true) // Secret Manager feature enabled by default
	FeatureFileManager   Feature = NewFeature("file_manager", true)   // File Manager feature enabled by default
)

// logFeatureStates logs the enabled/disabled state of all registered features.
func logFeatureStates() {
	for f, enabled := range features {
		slog.Info("feature state", "feature", f.String(), "enabled", enabled)
	}
}
