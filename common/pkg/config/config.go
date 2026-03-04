// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"
)

const (
	FinalizerSuffix = "finalizer"
)

// Configuration Loading Flow:
//
// 1. Parse --config flag (default: /etc/controlplane/config/config.yaml)
//    See: parseConfigPath() in flags.go
//
// 2. Load YAML file using ConfigSource interface
//    See: ConfigSource interface and FileSource in loader.go
//
// 3. Unmarshal into Config[T] with defaults applied
//    See: DefaultAppConfig[T]() in defaults.go
//
// 4. Validate using struct tags
//    See: Loader[T].validate() in loader.go
//
// 5. Compute derived values (e.g., FinalizerName)
//    See: ComputeValues() below
//
// 6. Store in global singleton for access via GetCommonConfig()
//    See: setCommonConfig() in global.go

// Config is the top-level controller configuration file structure.
//
// The YAML is split into:
// - common: shared controller-runtime/manager settings used by all controllers.
// - spec: component-specific configuration, defined by the importing controller.

type CommonConfig interface {
	CommonConfig() ControllerConfig
}
type Config[T any] struct {
	Common ControllerConfig `yaml:"common" validate:"required"`
	Spec   T                `yaml:"spec"`
}

func (config *Config[T]) CommonConfig() ControllerConfig {
	return config.Common
}

// ComputeValues applies computed values derived from other configuration fields.
// FinalizerName is computed from LabelKeyPrefix to ensure consistency.
func (config *Config[T]) ComputeValues() {
	config.Common.Reconciler.FinalizerName =
		config.Common.Reconciler.LabelKeyPrefix + "/" + FinalizerSuffix
}

// MetricsConfig configures the controller manager metrics endpoint.
type MetricsConfig struct {
	BindAddress   string `yaml:"bindAddress" validate:"required"`
	SecureServing bool   `yaml:"secureServing"`
}

// LeaderElectionConfig configures leader election for the controller manager.
//
// If Enabled is true, ID must be set by the component config to a unique value
// so multiple controllers do not fight over the same Lease.
type LeaderElectionConfig struct {
	Enabled bool   `yaml:"enabled"`
	ID      string `yaml:"id" validate:"required_if=Enabled true"`
}

// ProbeConfig configures the health/readiness probe bind address.
type ProbeConfig struct {
	BindAddress string `yaml:"bindAddress" validate:"required"`
}

// LogConfig configures controller logging behavior.
type LogConfig struct {
	Development bool `yaml:"development"`
}

// ControllerConfig contains shared controller-runtime manager settings.
//
// This struct maps to the YAML under the top-level "common" key.
type ControllerConfig struct {
	Metrics        MetricsConfig        `mapstructure:"metrics" validate:"required"`
	Probe          ProbeConfig          `mapstructure:"probe" validate:"required"`
	LeaderElection LeaderElectionConfig `mapstructure:"leaderElection" validate:"required"`
	Reconciler     ReconcilerConfig     `mapstructure:"reconciler" validate:"required"`
	EnableHTTP2    bool                 `mapstructure:"enableHTTP2"`
	Log            LogConfig            `mapstructure:"log"`
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

	DefaultNamespace   string `mapstructure:"default-namespace" validate:"required"`
	DefaultEnvironment string `mapstructure:"default-environment" validate:"required"`
	LabelKeyPrefix     string `mapstructure:"label-key-prefix" validate:"required"`
	FinalizerName      string `yaml:"-" mapstructure:"-"`
}
