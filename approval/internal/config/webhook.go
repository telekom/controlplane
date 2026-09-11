// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// WebhookConfig holds configuration for the approval webhook.
type WebhookConfig struct {
	// OperatorServiceAccount is the full Kubernetes username of the operator's service account.
	// Only this identity is permitted to transition an Approval to the Expired state.
	// Format: "system:serviceaccount:<namespace>:<name>"
	OperatorServiceAccount string
}

// LoadWebhookConfig reads webhook settings from environment variables prefixed with APPROVAL_.
func LoadWebhookConfig() (*WebhookConfig, error) {
	v := viper.New()
	v.SetEnvPrefix("APPROVAL")
	v.AutomaticEnv()

	v.SetDefault("operator_service_account", "system:serviceaccount:system:controller-manager")

	sa := v.GetString("operator_service_account")
	if sa == "" {
		return nil, fmt.Errorf("operator_service_account must not be empty")
	}

	return &WebhookConfig{
		OperatorServiceAccount: sa,
	}, nil
}
