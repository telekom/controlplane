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

type NotificationHousekeepingConfig struct {

	// TTLMonthsAfterFinished specifies how many months should the notification be kept in the system if it was successfully handled (all channels sent without error)
	TTLMonthsAfterFinished int32 `json:"ttlMonthsAfterFinished,omitempty"`
}

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
	// if true, emails will not be sent, just a log message will appear - should be used only for testing, to avoid spamming
	DryRun bool `mapstructure:"dryRun"`
}

func LoadHousekeepingConfig() (*NotificationHousekeepingConfig, error) {
	setHousekeepingConfigDefaults()

	var config NotificationHousekeepingConfig
	if err := viper.Unmarshal(&config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal notifications housekeeping config")
	}

	return &config, nil
}

func LoadEmailAdapterConfig() (*EmailAdapterConfig, error) {
	setEmailAdapterConfigDefaults()

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var config EmailAdapterConfig
	if err := viper.Unmarshal(&config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal email adapter config")
	}

	return &config, nil
}

func setEmailAdapterConfigDefaults() {
	viper.SetDefault("smtpConnection.host", "localhost")
	viper.SetDefault("smtpConnection.port", 25)
	viper.SetDefault("smtpConnection.user", "")
	viper.SetDefault("smtpConnection.password", "")

	viper.SetDefault("smtpSender.batchSize", 30)
	viper.SetDefault("smtpSender.batchLoopDelay", "1s")
	viper.SetDefault("smtpSender.maxRetries", 5)
	viper.SetDefault("smtpSender.initialBackoff", "1s")
	viper.SetDefault("smtpSender.maxBackoff", "1m")
	viper.SetDefault("smtpSender.defaultFrom", "email@telekom.de")
	viper.SetDefault("smtpSender.defaultName", "Team Tardis")
	viper.SetDefault("smtpSender.dryRun", false)
}

func setHousekeepingConfigDefaults() {
	viper.SetDefault("ttlMonthsAfterFinished", 2)
}
