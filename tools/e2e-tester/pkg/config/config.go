// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Case struct {
	Name        string         `mapstructure:"name"`
	Description string         `mapstructure:"description"` // Optional description of the test case purpose
	Type        string         `mapstructure:"type"`        // Command type: "roverctl" (default) or "snapshot"
	MustPass    bool           `mapstructure:"must_pass"`
	Command     string         `mapstructure:"command"`
	Compare     bool           `mapstructure:"compare"`
	Environment string         `mapstructure:"environment"` // Optional environment to run this case in
	Params      map[string]any `mapstructure:"params"`      // Optional type-specific parameters for future extensibility
	WaitBefore  time.Duration  `mapstructure:"wait_before"` // Optional wait time before executing the case
	WaitAfter   time.Duration  `mapstructure:"wait_after"`  // Optional wait time after executing the case
	Selector    string         `mapstructure:"selector"`    // YAML path selector for output processing
}

type Suite struct {
	Name         string   `mapstructure:"name"`
	Description  string   `mapstructure:"description"` // Optional description of the test suite purpose
	Cases        []*Case  `mapstructure:"cases"`
	Environments []string `mapstructure:"environments"` // Required list of environments to run this suite in
}

func (s *Suite) DeepCopy() *Suite {
	newCases := make([]*Case, len(s.Cases))
	for i, c := range s.Cases {
		newCase := *c
		newCases[i] = &newCase
	}

	newEnvs := make([]string, len(s.Environments))
	copy(newEnvs, s.Environments)

	return &Suite{
		Name:         s.Name,
		Description:  s.Description,
		Cases:        newCases,
		Environments: newEnvs,
	}
}

// GetName returns the formatted name of the suite
func (s Suite) GetName() string {
	if len(s.Environments) == 1 {
		return fmt.Sprintf("%s [%s]", s.Name, s.Environments[0]) // Use first environment for name consistency
	}
	if len(s.Environments) > 1 {
		return fmt.Sprintf("%s [%s]", s.Name, strings.Join(s.Environments, "_"))
	}
	return s.Name
}

type SnapshotterConfig struct {
	URL    string `mapstructure:"url"`
	Binary string `mapstructure:"binary"`
}

type RoverCtlConfig struct {
	DownloadURL string `mapstructure:"download_url"`
	Binary      string `mapstructure:"binary"`
}

type Environments struct {
	Name  string `mapstructure:"name"`
	Token string `mapstructure:"token"`
}

type Config struct {
	Snapshotter  SnapshotterConfig `mapstructure:"snapshotter"`
	RoverCtl     RoverCtlConfig    `mapstructure:"roverctl"`
	Environments []Environments    `mapstructure:"environments"`
	Suites       []Suite           `mapstructure:"suites"`
	Verbose      bool              `mapstructure:"verbose"`
}

// Validate checks if the config is valid
func (c *Config) Validate() error {
	// Validate suites
	if len(c.Suites) == 0 {
		return fmt.Errorf("at least one suite must be specified")
	}

	// Validate environments
	if len(c.Environments) == 0 {
		return fmt.Errorf("at least one environment must be specified")
	}

	// Ensure at least one case per suite and at least one environment per suite
	for _, suite := range c.Suites {
		if len(suite.Cases) == 0 {
			return fmt.Errorf("suite %s must have at least one case", suite.Name)
		}
	}

	return nil
}

// LoadConfig reads in config file
func LoadConfig(cfgFile string) error {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for default config names
		viper.SetConfigName("e2e-test-config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
	}

	// Read the config
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	zap.L().Info("Config loaded", zap.String("file", viper.ConfigFileUsed()))
	return nil
}
