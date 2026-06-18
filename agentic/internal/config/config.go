// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/spf13/viper"
)

// AgenticConfig holds agentic-domain-specific configuration.
type AgenticConfig struct {
	// TelecontextConsumerName is the consumer name of the Telecontext application.
	// Required for the TELECONTEXTMCP variant to auto-create ConsumeRoutes.
	TelecontextConsumerName string `mapstructure:"telecontext_consumer_name"`
}

// LoadConfig reads agentic configuration from environment variables
// prefixed with AGENTIC_ (e.g. AGENTIC_TELECONTEXT_CONSUMER_NAME).
func LoadConfig() (*AgenticConfig, error) {
	v := viper.New()
	v.SetEnvPrefix("AGENTIC")
	v.AutomaticEnv()

	v.SetDefault("telecontext_consumer_name", "")

	var cfg AgenticConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
