// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// AgenticConfig holds agentic-domain-specific configuration.
type AgenticConfig struct {
	// TelecontextApplicationID identifies the Telecontext Application in
	// "group--team--appName" format. Required for the TELECONTEXTMCP variant
	// to resolve the Telecontext Application's zone and consumer name.
	// The env var is AGENTIC_TELECONTEXT_APPLICATION_ID.
	TelecontextApplicationID string `mapstructure:"telecontext_application_id"`
}

// ParseTelecontextApplicationID splits the configured ID into its three parts.
// Returns group, team, appName or an error if the format is invalid.
func (c *AgenticConfig) ParseTelecontextApplicationID() (group, team, appName string, err error) {
	parts := strings.SplitN(c.TelecontextApplicationID, "--", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid telecontext application ID %q: expected format group--team--appName", c.TelecontextApplicationID)
	}
	return parts[0], parts[1], parts[2], nil
}

// LoadConfig reads agentic configuration from environment variables
// prefixed with AGENTIC_ (e.g. AGENTIC_TELECONTEXT_APPLICATION_ID).
func LoadConfig() (*AgenticConfig, error) {
	v := viper.New()
	v.SetEnvPrefix("AGENTIC")
	v.AutomaticEnv()

	v.SetDefault("telecontext_application_id", "")

	var cfg AgenticConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
