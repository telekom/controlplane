// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

const (
	ConfigKeyServerURL = "server.url"
	ConfigKeyTokenURL  = "token.url"

	ConfigKeyResourcePath = "resource.path"
	EnvResourcePath       = "RESOURCE_PATH"
)

// Initialize sets up viper for configuration management
func Initialize() {

	// Set default values
	setDefaults()

	// Setup environment variable support
	viper.SetEnvPrefix("ROVER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// RESOURCE_PATH is intentionally unprefixed.
	bindEnvOrDie(ConfigKeyResourcePath, EnvResourcePath)
}

// ResourcePath returns the optional base directory for relative --file paths.
func ResourcePath() string {
	return viper.GetString(ConfigKeyResourcePath)
}

func bindEnvOrDie(input ...string) {
	if err := viper.BindEnv(input...); err != nil {
		panic(fmt.Sprintf("binding env %q: %v", input, err))
	}
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
