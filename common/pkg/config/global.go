// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"sync"
)

const (
	LabelKeyPrefix  = "cp.ei.telekom.de"
	FinalizerSuffix = "finalizer"
	FinalizerName   = LabelKeyPrefix + "/" + FinalizerSuffix
)

// EmptySpec is a placeholder spec type for controllers that do not need any
// component-specific configuration.
type EmptySpec struct{}

func EmptySpecDefault() EmptySpec {
	return EmptySpec{}
}

var once sync.Once

var commonConfig *ControllerConfig

const configPath = "/etc/controlplane/config/config.yaml"

func GetCommonConfig() ControllerConfig {
	// If config has not been loaded yet, load
	if commonConfig == nil {
		c := defaultControllerConfig()
		commonConfig = &c
	}
	return *commonConfig
}

func setCommonConfig(cfg *ControllerConfig) {
	commonConfig = cfg
}

// LoadWithTemplate loads configuration from the file path.
// It applies defaults, validates, and stores the common config in a global singleton.
func LoadWithTemplate[T any](defaulter func() T) (*Config[T], error) {
	ctx := context.Background()
	cfg, err := LoadFromFile[T](ctx, configPath, defaulter)
	if err != nil {
		return nil, err
	}

	setCommonConfig(&cfg.Common)
	return cfg, nil
}

// LoadOrDieWithTemplate loads configuration and panics if an error occurs.
// Use this in main() where startup failure is acceptable.
func LoadOrDieWithTemplate[T any](defaulter func() T) *Config[T] {
	cfg, err := LoadWithTemplate[T](defaulter)
	if err != nil {
		panic(err)
	}
	return cfg
}

// Load() simplified handler for domains without seperate configuration
func Load() (CommonConfig, error) {
	return LoadWithTemplate[EmptySpec](EmptySpecDefault)
}

// LoadOrDie: like Load(), but panics if error
func LoadOrDie() CommonConfig {
	return LoadOrDieWithTemplate[EmptySpec](EmptySpecDefault)
}
