// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import "time"

// DefaultControllerConfig returns the default controller configuration.
// These values are used as fallbacks when configuration files do not specify them.
func DefaultControllerConfig() ControllerConfig {
	return ControllerConfig{
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
}

// DefaultAppConfig returns a Config[T] with default values.
func DefaultAppConfig[T any]() Config[T] {
	return Config[T]{
		Common: DefaultControllerConfig(),
	}
}
