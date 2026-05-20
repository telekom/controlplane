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

// LoadExpirationConfig loads the expiration configuration from environment variables
func LoadExpirationConfig() (*ExpirationConfig, error) {
	setExpirationConfigDefaults()

	viper.AutomaticEnv()
	viper.SetEnvPrefix("APPROVAL")

	expirationMonths := viper.GetInt("EXPIRATION_PERIOD_MONTHS")
	weeklyReminderMonths := viper.GetInt("EXPIRATION_WEEKLY_REMINDER_MONTHS")
	dailyReminderWeeks := viper.GetInt("EXPIRATION_DAILY_REMINDER_WEEKS")

	if expirationMonths <= 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_PERIOD_MONTHS must be greater than 0")
	}
	if weeklyReminderMonths < 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_WEEKLY_REMINDER_MONTHS must be >= 0")
	}
	if dailyReminderWeeks < 0 {
		return nil, errors.New("APPROVAL_EXPIRATION_DAILY_REMINDER_WEEKS must be >= 0")
	}

	// Convert to durations
	expirationDuration := time.Duration(expirationMonths) * 30 * 24 * time.Hour

	// Build default thresholds
	var thresholds []reminder.Threshold

	// Weekly reminder threshold (if configured)
	if weeklyReminderMonths > 0 {
		weeklyBefore := time.Duration(weeklyReminderMonths) * 30 * 24 * time.Hour
		thresholds = append(thresholds, reminder.Threshold{
			Before: metav1.Duration{Duration: weeklyBefore},
		})
	}

	// Daily reminder threshold (if configured)
	if dailyReminderWeeks > 0 {
		dailyBefore := time.Duration(dailyReminderWeeks) * 7 * 24 * time.Hour
		dailyRepeat := metav1.Duration{Duration: 24 * time.Hour}
		thresholds = append(thresholds, reminder.Threshold{
			Before: metav1.Duration{Duration: dailyBefore},
			Repeat: &dailyRepeat,
		})
	}

	return &ExpirationConfig{
		ExpirationDuration: expirationDuration,
		DefaultThresholds:  thresholds,
	}, nil
}

func setExpirationConfigDefaults() {
	viper.SetDefault("EXPIRATION_PERIOD_MONTHS", 12)
	viper.SetDefault("EXPIRATION_WEEKLY_REMINDER_MONTHS", 1)
	viper.SetDefault("EXPIRATION_DAILY_REMINDER_WEEKS", 1)
}
