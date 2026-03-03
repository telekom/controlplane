// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controllerconfig

import (
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

const (
	FinalizerSuffix = "finalizer"
)

// AppConfig is the top-level controller configuration file structure.
//
// The YAML is split into:
// - common: shared controller-runtime/manager settings used by all controllers.
// - spec: component-specific configuration, defined by the importing controller.
type AppConfig[T any] struct {
	Common ControllerConfig `yaml:"common" validate:"required"`
	Spec   T                `yaml:"spec"`
}

// ComputeValues apply common computed values
func (config *AppConfig[T]) ComputeValues() {
	config.Common.Reconciler.FinalizerName =
		config.Common.Reconciler.LabelKeyPrefix + "/" + FinalizerSuffix
}

// EmptySpec is a placeholder spec type for controllers that do not need any
// component-specific configuration.
type EmptySpec struct{}

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

func defaultControllerConfig() ControllerConfig {
	controllerConfig := ControllerConfig{
		Metrics: MetricsConfig{
			BindAddress:   "0",
			SecureServing: true,
		},
		Probe: ProbeConfig{
			BindAddress: ":8081",
		},
		LeaderElection: LeaderElectionConfig{
			Enabled: false,
			ID:      "",
		},
		EnableHTTP2: false,
		Reconciler: ReconcilerConfig{
			RequeueAfterOnError:     1 * time.Second,
			RequeueAfter:            30 * time.Minute,
			JitterFactor:            0.7,
			MaxBackoff:              5 * time.Minute,
			MaxConcurrentReconciles: 10,

			DefaultNamespace:   "default",
			DefaultEnvironment: "default",
			LabelKeyPrefix:     "cp.ei.telekom.de",
		},
		Log: LogConfig{
			Development: true,
		},
	}
	return controllerConfig
}

func defaultAppConfig[T any]() AppConfig[T] {
	return AppConfig[T]{
		Common: defaultControllerConfig(),
	}
}

func loadConfigFromFile[T any](path string) (*AppConfig[T], error) {
	cfg := defaultAppConfig[T]()

	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, err
	}

	val := validator.New(validator.WithRequiredStructEnabled())
	if err := val.Struct(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfg.ComputeValues()

	return &cfg, nil
}
