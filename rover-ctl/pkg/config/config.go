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
	viper.SetDefault("server.baseUrl", "/rover/api")

	// Logging defaults
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "console")
	viper.SetDefault("output.format", "yaml")

	// Authentication defaults
	viper.SetDefault("token", "") // ROVER_TOKEN
	viper.SetDefault(ConfigKeyTokenURL, "")
	viper.SetDefault("access.token", "") // ROVER_ACCESS_TOKEN (used only for local testing)

	// Polling defaults
	viper.SetDefault("timeout.status", "30s")         // ROVER_TIMEOUT_STATUS
	viper.SetDefault("timeout.secretRotation", "30s") // ROVER_TIMEOUT_SECRET_ROTATION
	viper.SetDefault("poll.interval", "1s")           // ROVER_POLL_INTERVAL
}
