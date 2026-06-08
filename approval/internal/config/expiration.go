// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/reminder"
)

// ExpirationConfig holds the configuration for approval expiration
type ExpirationConfig struct {
	// ExpirationDuration is how long until an approval expires (from creation)
	ExpirationDuration time.Duration

	// DefaultThresholds defines the reminder schedule for approvals
	DefaultThresholds []reminder.Threshold
}

// rawExpirationConfig is the intermediate struct used to unmarshal environment variables.
// Each field maps to an env var with the APPROVAL_ prefix (e.g. APPROVAL_EXPIRATION_PERIOD_MONTHS).
type rawExpirationConfig struct {
	ExpirationPeriodMonths int `mapstructure:"expiration_period_months"`
	WeeklyReminderMonths   int `mapstructure:"expiration_weekly_reminder_months"`
	DailyReminderWeeks     int `mapstructure:"expiration_daily_reminder_weeks"`
}

// LoadExpirationConfig loads the expiration configuration from environment variables.
// Uses a local Viper instance to avoid interfering with the global Viper configuration
// used by common/pkg/config.
func LoadExpirationConfig() (*ExpirationConfig, error) {
	v := viper.New()
	v.SetEnvPrefix("APPROVAL")
	v.AutomaticEnv()

	v.SetDefault("expiration_period_months", 12)
	v.SetDefault("expiration_weekly_reminder_months", 1)
	v.SetDefault("expiration_daily_reminder_weeks", 1)

	var raw rawExpirationConfig
	if err := v.Unmarshal(&raw); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal expiration config")
	}

	if raw.ExpirationPeriodMonths <= 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_PERIOD_MONTHS must be greater than 0")
	}
	if raw.WeeklyReminderMonths < 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_WEEKLY_REMINDER_MONTHS must be >= 0")
	}
	if raw.DailyReminderWeeks < 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_DAILY_REMINDER_WEEKS must be >= 0")
	}

	expirationDuration := time.Duration(raw.ExpirationPeriodMonths) * 30 * 24 * time.Hour

	var thresholds []reminder.Threshold
	if raw.WeeklyReminderMonths > 0 {
		thresholds = append(thresholds, reminder.Threshold{
			Before: metav1.Duration{Duration: time.Duration(raw.WeeklyReminderMonths) * 30 * 24 * time.Hour},
		})
	}
	if raw.DailyReminderWeeks > 0 {
		dailyRepeat := metav1.Duration{Duration: 24 * time.Hour}
		thresholds = append(thresholds, reminder.Threshold{
			Before: metav1.Duration{Duration: time.Duration(raw.DailyReminderWeeks) * 7 * 24 * time.Hour},
			Repeat: &dailyRepeat,
		})
	}

	return &ExpirationConfig{
		ExpirationDuration: expirationDuration,
		DefaultThresholds:  thresholds,
	}, nil
}
