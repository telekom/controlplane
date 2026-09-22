// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LevelEnvVar is the environment variable operators can set to configure the
// controller's log verbosity (e.g. "debug", "info", "warn", "error").
const LevelEnvVar = "LOG_LEVEL"

// LevelFromEnv reads LevelEnvVar and returns a zap.AtomicLevel suitable for
// zap.Options.Level. If the variable is unset, empty, or does not match a
// recognized zap level name, it defaults to zapcore.InfoLevel.
func LevelFromEnv() zap.AtomicLevel {
	return LevelFromValue(os.Getenv(LevelEnvVar))
}

// LevelFromValue parses value into a zap.AtomicLevel, defaulting to
// zapcore.InfoLevel when value is empty or does not match a recognized zap
// level name (e.g. "debug", "info", "warn", "error"; case-insensitive).
//
// It is exported separately from LevelFromEnv so callers/tests can exercise
// the parsing logic without mutating the process environment.
func LevelFromValue(value string) zap.AtomicLevel {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
		lvl = zapcore.InfoLevel
	}
	return zap.NewAtomicLevelAt(lvl)
}
