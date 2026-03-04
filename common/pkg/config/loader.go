// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// ConfigSource defines the interface for loading configuration data.
// Implementations can load from files, ConfigMaps, or other sources.
type ConfigSource interface {
	// Load returns the raw configuration bytes.
	Load(ctx context.Context) ([]byte, error)
}

// FileSource loads configuration from a file path.
type FileSource struct {
	Path string
}

// Load reads the configuration file and returns its contents.
func (fs *FileSource) Load(ctx context.Context) ([]byte, error) {
	v := viper.New()
	v.SetConfigFile(fs.Path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Get the raw config data
	allSettings := v.AllSettings()
	if len(allSettings) == 0 {
		return nil, fmt.Errorf("config file is empty: %s", fs.Path)
	}

	return nil, nil // viper handles the parsing internally
}

// Loader handles loading and validating configuration from a ConfigSource.
type Loader[T any] struct {
	source ConfigSource
}

// NewLoader creates a new Loader with the given ConfigSource.
func NewLoader[T any](source ConfigSource) *Loader[T] {
	return &Loader[T]{source: source}
}

// Load loads configuration from the source, applies defaults, validates, and computes derived values.
func (l *Loader[T]) Load(ctx context.Context) (*Config[T], error) {
	cfg := DefaultAppConfig[T]()

	// Load from source
	if err := l.loadFromSource(ctx, &cfg); err != nil {
		return nil, err
	}

	// Validate configuration
	if err := l.validate(&cfg); err != nil {
		return nil, err
	}

	// Compute derived values
	cfg.ComputeValues()

	return &cfg, nil
}

// loadFromSource loads configuration from the ConfigSource into cfg.
// For FileSource, it uses viper to parse YAML.
func (l *Loader[T]) loadFromSource(ctx context.Context, cfg *Config[T]) error {
	fileSource, ok := l.source.(*FileSource)
	if !ok {
		return fmt.Errorf("unsupported config source type")
	}

	v := viper.New()
	v.SetConfigFile(fileSource.Path)
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := v.UnmarshalExact(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// validate validates the configuration structure and required fields.
func (l *Loader[T]) validate(cfg *Config[T]) error {
	val := validator.New(validator.WithRequiredStructEnabled())
	if err := val.Struct(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return nil
}

// LoadFromFile is a convenience function that loads configuration from a file path.
func LoadFromFile[T any](path string) (*Config[T], error) {
	source := &FileSource{Path: path}
	loader := NewLoader[T](source)
	return loader.Load(context.Background())
}
