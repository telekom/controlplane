// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const DefaultConfigPath = "/etc/controlplane/config/config.yaml"

const configKey = "config"

// parseConfigPath parses the only allowed CLI flag: --config.
// It uses viper for flag access (no env, no config file merging).
func parseConfigPath() (string, error) {
	v := viper.New()

	fs := pflag.NewFlagSet("controller", pflag.ContinueOnError)
	fs.String(configKey, DefaultConfigPath, "Path to the YAML config file")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return "", err
	}
	if err := v.BindPFlag(configKey, fs.Lookup(configKey)); err != nil {
		return "", err
	}

	return v.GetString(configKey), nil
}

// Load loads configuration from the file path specified by the --config flag.
// It applies defaults, validates, and stores the common config in a global singleton.
func Load[T any]() (*Config[T], error) {
	configPath, err := parseConfigPath()
	if err != nil {
		return nil, err
	}

	cfg, err := LoadFromFile[T](configPath)
	if err != nil {
		return nil, err
	}

	setCommonConfig(cfg)
	return cfg, nil
}

// LoadOrDie loads configuration and panics if an error occurs.
// Use this in main() where startup failure is acceptable.
func LoadOrDie[T any]() *Config[T] {
	cfg, err := Load[T]()
	if err != nil {
		panic(err)
	}
	return cfg
}
