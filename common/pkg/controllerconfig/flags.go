// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controllerconfig

import (
	"os"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const DefaultConfigPath = "/etc/controlplane/config/config.yaml"

const configKey = "config"

// parseConfigPathFromFlags parses the only allowed CLI flag: --config.
// It uses viper for flag access (no env, no config file merging).
func parseConfigPathFromFlags() (string, error) {
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

func Load[T any]() (*AppConfig[T], error) {
	configPath, err := parseConfigPathFromFlags()
	if err != nil {
		return nil, err
	}
	return loadConfigFromFile[T](configPath)
}

func LoadOrDie[T any]() *AppConfig[T] {
	cfg, err := Load[T]()
	if err != nil {
		panic(err)
	}
	return cfg
}
