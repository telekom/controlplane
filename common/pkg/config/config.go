// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	_ "github.com/go-playground/validator/v10"
	"time"
)

type CommonConfig interface {
	GetCommon() ControllerConfig
}

type Config[T any] struct {
	Common ControllerConfig `mapstructure:"common" validate:"required"`
	Spec   T                `mapstructure:"spec"`
}

func (cfg *Config[T]) GetCommon() ControllerConfig {
	return cfg.Common
}

// MetricsConfig configures the controller manager metrics endpoint.
type MetricsConfig struct {
	// BindAddress: The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.
	BindAddress string `mapstructure:"bindAddress" validate:"required"`
	// SecureServing: If set, the metrics endpoint is served securely via HTTPS.
	SecureServing bool       `mapstructure:"secureServing"`
	Cert          CertConfig `mapstructure:"cert" validate:"required_if=secureServing true"`
}

type CertConfig struct {
	// The directory that contains the server certificate file
	Path string `mapstructure:"path"`
	// The name of the server certificate file
	Name string `mapstructure:"name"`
	// The name of the server key file
	Key string `mapstructure:"key"`
}

type WebhookConfig struct {
	Cert CertConfig `mapstructure:"cert"`
}

// ProbeConfig configures the health/readiness probe bind address.
type ProbeConfig struct {
	// BindAddress: The address the health/readiness probe endpoint binds to.
	BindAddress string `mapstructure:"bindAddress" validate:"required"`
}

// LogConfig configures controller logging behavior.
type LogConfig struct {
	// Development: If set, the controller manager will run in development mode.
	Development bool `mapstructure:"development"`
}

// ControllerConfig contains shared controller-runtime manager settings.
//
// This struct maps to the YAML under the top-level "common" key.
type ControllerConfig struct {
	Metrics    MetricsConfig    `mapstructure:"metrics" validate:"required"`
	Webhook    WebhookConfig    `mapstructure:"webhook" validate:"required"`
	Probe      ProbeConfig      `mapstructure:"probe" validate:"required"`
	Reconciler ReconcilerConfig `mapstructure:"reconciler" validate:"required"`
	// EnableHTTP2: If set, HTTP/2 will be enabled for the metrics and webhook servers
	EnableHTTP2 bool            `mapstructure:"enableHTTP2"`
	Log         LogConfig       `mapstructure:"log"`
	Features    []FeatureConfig `mapstructure:"features"`
}

type ReconcilerConfig struct {
	// RequeueAfterOnError is the time to wait before retrying a failed operation.
	// This applies for all controller errors.
	RequeueAfterOnError time.Duration `mapstructure:"requeue-after-on-error" validate:"required"`
	// RequeueAfter is the time to wait before retrying a successful operation.
	RequeueAfter time.Duration `mapstructure:"requeue-after" validate:"required"`
	// JitterFactor is the factor to apply to the backoff duration.
	JitterFactor float64 `mapstructure:"jitter-factor" validate:"required"`
	// MaxBackoff is the maximum backoff duration.
	MaxBackoff time.Duration `mapstructure:"max-backoff" validate:"required"`
	// MaxConcurrentReconciles is the maximum number of concurrent reconciles.
	MaxConcurrentReconciles int `mapstructure:"max-concurrent-reconciles" validate:"required"`
}

type FeatureConfig struct {
	Name    string `mapstructure:"name" validate:"required"`
	Enabled bool   `mapstructure:"enabled"`
}
