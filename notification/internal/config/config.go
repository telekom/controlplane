// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"strings"
)

// EmailAdapterConfig wrapper for the static config of the mail notification adapter
type EmailAdapterConfig struct {
	SMTPHost string `json:"smtpHost"`
	SMTPPort int    `json:"smtpPort"`
}

func LoadEmailAdapterConfig() (*EmailAdapterConfig, error) {

	setDefaults()

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var config EmailAdapterConfig
	if err := viper.Unmarshal(&config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("SMTPHost", "localhost")
	viper.SetDefault("SMTPPort", 25)
}
