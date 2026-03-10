// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"
)

type NotificationConfig struct {
	HouseKeeping NotificationHousekeepingConfig `mapstructure:"notification"`
	EmailAdapter EmailAdapterConfig             `mapstructure:"email"`
}

func DefaultNotificationConfig() NotificationConfig {
	return NotificationConfig{
		HouseKeeping: NotificationHousekeepingConfig{
			TTLMonthsAfterFinished: 2,
		},
		EmailAdapter: EmailAdapterConfig{
			SMTPConnection: SMTPConnection{
				Host:     "localhost",
				Port:     25,
				User:     "",
				Password: "",
			},
			SMTPSender: SMTPSender{
				BatchSize:      30,
				MaxRetries:     5,
				InitialBackoff: 1 * time.Second,
				MaxBackoff:     1 * time.Minute,
				BatchLoopDelay: 1 * time.Second,
				DefaultFrom:    "email@telekom.de",
				DefaultName:    "Team Controlplane",
				DryRun:         false,
			},
		},
	}
}

type NotificationHousekeepingConfig struct {

	// TTLMonthsAfterFinished specifies how many months should the notification be kept in the system if it was successfully handled (all channels sent without error)
	TTLMonthsAfterFinished int32 `json:"ttlMonthsAfterFinished,omitempty" mapstructure:"ttlMonthsAfterFinished"`
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
