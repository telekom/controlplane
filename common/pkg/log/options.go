// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// DefaultOptions returns the baseline zap.Options used across controller
// entrypoints: JSON-encoded, non-development output with the log level
// configured via the LOG_LEVEL environment variable (see LevelFromEnv),
// defaulting to Info.
//
// Callers can override any field on the returned struct before passing it to
// zap.New/UseFlagOptions (e.g. to enable Development mode locally), and can
// still call Options.BindFlags to allow CLI flags such as --zap-log-level to
// take precedence.
func DefaultOptions() zap.Options {
	return zap.Options{
		Development: false,
		Level:       LevelFromEnv(),
	}
}
