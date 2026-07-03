// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	// slog is used intentionally here instead of zap/zapr because this code runs
	// during init(), before any structured logger (e.g. zap) is initialized.
	// Using the stdlib slog avoids introducing a dependency on logger initialization order.
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
	FeaturePermission    Feature = NewFeature("permission", false)    // Permission feature disabled by default
	FeatureSecretManager Feature = NewFeature("secret_manager", true) // Secret Manager feature enabled by default
	FeatureFileManager   Feature = NewFeature("file_manager", true)   // File Manager feature enabled by default
	// TODO(DHEI-20905): File/SFTP domain feature disabled by default. The rover-domain
	// dispatch is in place, but the File domain (DHEI-20903) and SFTP/DDS domain
	// (DHEI-20904) are not yet available. Enable this flag once those domains land so
	// the rover operator can create file-domain resources instead of returning Blocked.
	FeatureFile Feature = NewFeature("file", false)
)

// SetFeatureEnabled sets the enabled state for a feature. Intended for tests.
func SetFeatureEnabled(f Feature, enabled bool) {
	features[f] = enabled
}

// logFeatureStates logs the enabled/disabled state of all registered features.
func logFeatureStates() {
	for f, enabled := range features {
		slog.Info("feature state", "feature", f.String(), "enabled", enabled)
	}
}
