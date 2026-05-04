// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// ExpirationConfig holds the configuration for approval expiration
type ExpirationConfig struct {
	// ExpirationPeriodMonths is the number of months until an approval expires
	ExpirationPeriodMonths int

	// LastMonthsWithWeeklyReminder is the number of months before expiration when weekly reminders start
	LastMonthsWithWeeklyReminder int

	// LastWeeksWithDailyReminder is the number of weeks before expiration when daily reminders start
	LastWeeksWithDailyReminder int
}

// LoadExpirationConfig loads the expiration configuration from environment variables
func LoadExpirationConfig() (*ExpirationConfig, error) {
	setExpirationConfigDefaults()

	viper.AutomaticEnv()
	viper.SetEnvPrefix("APPROVAL")

	config := &ExpirationConfig{
		ExpirationPeriodMonths:       viper.GetInt("EXPIRATION_PERIOD_MONTHS"),
		LastMonthsWithWeeklyReminder: viper.GetInt("EXPIRATION_WEEKLY_REMINDER_MONTHS"),
		LastWeeksWithDailyReminder:   viper.GetInt("EXPIRATION_DAILY_REMINDER_WEEKS"),
	}

	if config.ExpirationPeriodMonths <= 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_PERIOD_MONTHS must be greater than 0")
	}
	if config.LastMonthsWithWeeklyReminder < 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_WEEKLY_REMINDER_MONTHS must be >= 0")
	}
	if config.LastWeeksWithDailyReminder < 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_DAILY_REMINDER_WEEKS must be >= 0")
	}

	return config, nil
}

func setExpirationConfigDefaults() {
	viper.SetDefault("EXPIRATION_PERIOD_MONTHS", 12)
	viper.SetDefault("EXPIRATION_WEEKLY_REMINDER_MONTHS", 2)
	viper.SetDefault("EXPIRATION_DAILY_REMINDER_WEEKS", 2)
}
