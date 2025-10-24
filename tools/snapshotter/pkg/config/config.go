// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/decoder"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/obfuscator"
	"k8s.io/client-go/util/homedir"
)

type SourceConfig struct {
	Environment  string                         `mapstructure:"environment" validate:"required"`
	Zone         string                         `mapstructure:"zone" validate:"required"`
	Url          string                         `mapstructure:"url" validate:"required,url"`
	TokenUrl     string                         `mapstructure:"tokenUrl" validate:"required,url"`
	ClientId     string                         `mapstructure:"clientId" validate:"required"`
	ClientSecret string                         `mapstructure:"clientSecret" validate:"required"`
	Scopes       []string                       `mapstructure:"scopes"`
	Tags         []string                       `mapstructure:"tags"`
	Obfuscators  []obfuscator.ObfuscationTarget `mapstructure:"obfuscators"`
	Decoders     []decoder.DecoderTarget        `mapstructure:"decoders"`
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
	Obfuscators []obfuscator.ObfuscationTarget `mapstructure:"obfuscators"`
	Decoders    []decoder.DecoderTarget        `mapstructure:"decoders"`
	Sources     map[string]SourceConfig        `mapstructure:"sources"`
}

func (c *Config) GetSourceConfig(name string) (SourceConfig, bool) {
	source, exists := c.Sources[name]
	if !exists {
		return SourceConfig{}, false
	}

	// Merge global config
	source.Obfuscators = append(source.Obfuscators, c.Obfuscators...)
	source.Decoders = append(source.Decoders, c.Decoders...)
	return source, true
}

func (c *Config) GetSourceConfigs() map[string]SourceConfig {
	result := make(map[string]SourceConfig)
	for name := range c.Sources {
		if source, exists := c.GetSourceConfig(name); exists {
			result[name] = source
		}
	}
	return result
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

	err := validator.New().Struct(config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}
