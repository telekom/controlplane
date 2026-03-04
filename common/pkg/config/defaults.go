// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import "time"

// defaultControllerConfig returns the default controller configuration.
// These values are used as fallbacks when configuration files do not specify them.
func defaultControllerConfig() ControllerConfig {
	return ControllerConfig{
		Metrics: MetricsConfig{
			BindAddress:   "0",
			SecureServing: true,
			Cert: CertConfig{
				Path: "",
				Name: "tls.crt",
				Key:  "tls.key",
			},
		},
		Webhook: WebhookConfig{
			Cert: CertConfig{
				Path: "",
				Name: "tls.crt",
				Key:  "tls.key",
			},
		},
		Probe: ProbeConfig{
			BindAddress: ":8081",
		},
		EnableHTTP2: false,
		Reconciler: ReconcilerConfig{
			RequeueAfterOnError:     1 * time.Second,
			RequeueAfter:            30 * time.Minute,
			JitterFactor:            0.7,
			MaxBackoff:              5 * time.Minute,
			MaxConcurrentReconciles: 10,
		},
		Log: LogConfig{
			Development: true,
		},
	}
}

// defaultConfig returns a Config[T] with default values.
// factory should return a zero-initialized instance of T with any defaults applied.
func defaultConfig[T any](defaulter func() T) Config[T] {
	return Config[T]{
		Common: defaultControllerConfig(),
		Spec:   defaulter(),
	}
}
