// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/obfuscator"
	"k8s.io/client-go/util/homedir"
)

var (
	GlobalObfuscationTargets = []obfuscator.ObfuscationTarget{
		{
			Pattern: `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`,
			Replace: `00000000-0000-0000-0000-000000000000`,
		}, {
			Pattern: `[0-9]{10}`,
			Replace: `0`,
		},
	}
)

type SourceConfig struct {
	Environment  string                         `mapstructure:"environment"`
	Zone         string                         `mapstructure:"zone"`
	Url          string                         `mapstructure:"url"`
	TokenUrl     string                         `mapstructure:"tokenUrl"`
	ClientId     string                         `mapstructure:"clientId"`
	ClientSecret string                         `mapstructure:"clientSecret"`
	Scopes       []string                       `mapstructure:"scopes"`
	Tags         []string                       `mapstructure:"tags"`
	Obfuscators  []obfuscator.ObfuscationTarget `mapstructure:"obfuscators"`
}

// AdminClientId implements kongutil.GatewayAdminConfig.
func (s SourceConfig) AdminClientId() string {
	return s.ClientId
}

// AdminClientSecret implements kongutil.GatewayAdminConfig.
func (s SourceConfig) AdminClientSecret() string {
	return s.ClientSecret
}

// AdminIssuer implements kongutil.GatewayAdminConfig.
func (s SourceConfig) AdminIssuer() string {
	return s.TokenUrl
}

// AdminUrl implements kongutil.GatewayAdminConfig.
func (s SourceConfig) AdminUrl() string {
	return s.Url
}

type Config struct {
	Sources map[string]SourceConfig
}

func LoadConfig(path string) (Config, error) {

	viper.AutomaticEnv()

	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.SetConfigName("snapshotter-config")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
		viper.AddConfigPath(homedir.HomeDir())
	}

	var config Config

	if err := viper.ReadInConfig(); err != nil {
		return Config{}, err
	}

	if err := viper.Unmarshal(&config); err != nil {
		return Config{}, err
	}

	return config, nil
}
