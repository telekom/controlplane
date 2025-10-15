// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"strings"
	"time"
)

// EmailAdapterConfig wrapper for the static config of the mail notification adapter
type EmailAdapterConfig struct {
	SMTPConnection SMTPConnection `mapstructure:"smtpConnection"`
	SMTPSender     SMTPSender     `mapstructure:"smtpSender"`
}

type SMTPConnection struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

type SMTPSender struct {
	BatchSize      int           `mapstructure:"batchSize"`
	MaxRetries     int           `mapstructure:"maxRetries"`
	InitialBackoff time.Duration `mapstructure:"initialBackoff"`
	MaxBackoff     time.Duration `mapstructure:"maxBackoff"`
	BatchLoopDelay time.Duration `mapstructure:"batchLoopDelay"`
	DefaultFrom    string        `mapstructure:"defaultFrom"`
	DefaultName    string        `mapstructure:"defaultName"`
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
	viper.SetDefault("smtpConnection.host", "smtp.dev.dhei.telekom.de")
	viper.SetDefault("smtpConnection.port", 465)
	viper.SetDefault("smtpConnection.user", "")
	viper.SetDefault("smtpConnection.password", "")

	viper.SetDefault("smtpSender.batchSize", 30)
	viper.SetDefault("smtpSender.batchLoopDelay", "1s")
	viper.SetDefault("smtpSender.maxRetries", 5)
	viper.SetDefault("smtpSender.initialBackoff", "1s")
	viper.SetDefault("smtpSender.maxBackoff", "1m")
	viper.SetDefault("smtpSender.defaultFrom", "noreply_fmb_tardis_support@telekom.de")
	viper.SetDefault("smtpSender.defaultName", "Team Tardis")
}
