// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"

	"github.com/spf13/viper"
)

const (
	ConfigKeyServerURL = "server.url"
	ConfigKeyTokenURL  = "token.url"
)

// Initialize sets up viper for configuration management
func Initialize() {

	// Set default values
	setDefaults()

	// Setup environment variable support
	viper.SetEnvPrefix("ROVER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
}

// setDefaults sets default values for all configuration options
func setDefaults() {
	// Server defaults
	viper.SetDefault(ConfigKeyServerURL, "")

	// Logging defaults
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "console")
	viper.SetDefault("output.format", "yaml")

	// Authentication defaults
	viper.SetDefault("token", "") // ROVER_TOKEN
	viper.SetDefault(ConfigKeyTokenURL, "")
}
