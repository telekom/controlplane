// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/reminder"
)

// ExpirationConfig holds the resolved expiration settings.
type ExpirationConfig struct {
	// ExpirationDuration is how long until an approval expires (from when it is granted).
	ExpirationDuration time.Duration

	// DefaultThresholds are the reminder thresholds copied into each new ApprovalExpiration.
	DefaultThresholds []reminder.Threshold
}

// rawExpirationConfig is the intermediate struct used for env-var unmarshalling.
type rawExpirationConfig struct {
	ExpirationPeriodMonths int `mapstructure:"expiration_period_months"`
	WeeklyReminderMonths   int `mapstructure:"expiration_weekly_reminder_months"`
	DailyReminderWeeks     int `mapstructure:"expiration_daily_reminder_weeks"`
}

// LoadExpirationConfig reads expiration settings from environment variables
// prefixed with APPROVAL_ and returns a validated ExpirationConfig.
func LoadExpirationConfig() (*ExpirationConfig, error) {
	v := viper.New()
	v.SetEnvPrefix("APPROVAL")
	v.AutomaticEnv()

	v.SetDefault("expiration_period_months", 12)
	v.SetDefault("expiration_weekly_reminder_months", 1)
	v.SetDefault("expiration_daily_reminder_weeks", 1)

	var raw rawExpirationConfig
	if err := v.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal expiration config: %w", err)
	}

	if raw.ExpirationPeriodMonths <= 0 {
		return nil, fmt.Errorf("expiration_period_months must be > 0, got %d", raw.ExpirationPeriodMonths)
	}
	if raw.WeeklyReminderMonths < 0 {
		return nil, fmt.Errorf("expiration_weekly_reminder_months must be >= 0, got %d", raw.WeeklyReminderMonths)
	}
	if raw.DailyReminderWeeks < 0 {
		return nil, fmt.Errorf("expiration_daily_reminder_weeks must be >= 0, got %d", raw.DailyReminderWeeks)
	}

	expirationDuration := time.Duration(raw.ExpirationPeriodMonths) * 30 * 24 * time.Hour

	thresholds := []reminder.Threshold{
		{
			Before: metav1.Duration{Duration: time.Duration(raw.WeeklyReminderMonths) * 30 * 24 * time.Hour},
		},
		{
			Before: metav1.Duration{Duration: time.Duration(raw.DailyReminderWeeks) * 7 * 24 * time.Hour},
			Repeat: &metav1.Duration{Duration: 24 * time.Hour},
		},
	}

	return &ExpirationConfig{
		ExpirationDuration: expirationDuration,
		DefaultThresholds:  thresholds,
	}, nil
}
