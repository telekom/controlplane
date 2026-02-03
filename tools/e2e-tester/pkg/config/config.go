// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"path"
	"strings"
	"time"

	"slices"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// RunPolicy defines how a test case behaves relative to prior test failures.
type RunPolicy string

const (
	// RunPolicyNormal - test runs when prior tests passed, skipped when prior failed.
	RunPolicyNormal RunPolicy = "normal"

	// RunPolicyCritical - test runs when prior passed, aborts suite on ERROR status.
	RunPolicyCritical RunPolicy = "critical"

	// RunPolicyAlways - test always runs regardless of prior failures (for cleanup).
	RunPolicyAlways RunPolicy = "always"
)

// ValidRunPolicies contains all valid RunPolicy values for validation.
var ValidRunPolicies = []RunPolicy{RunPolicyNormal, RunPolicyCritical, RunPolicyAlways}

// IsValid checks if a RunPolicy value is valid.
func (p RunPolicy) IsValid() bool {
	return slices.Contains(ValidRunPolicies, p)
}

type Case struct {
	Name        string         `mapstructure:"name" validate:"required"`
	Description string         `mapstructure:"description"`                                                             // Optional description of the test case purpose
	Type        string         `mapstructure:"type" validate:"omitempty,oneof=roverctl snapshot"`                       // Command type: "roverctl" (default) or "snapshot"
	RunPolicy   RunPolicy      `mapstructure:"run_policy" validate:"omitempty,run_policy,oneof=normal critical always"` // Execution policy: "normal" (default), "critical", "always"
	Command     string         `mapstructure:"command" validate:"required"`
	Compare     bool           `mapstructure:"compare"`
	Environment string         `mapstructure:"environment"`                            // Optional environment to run this case in
	Params      map[string]any `mapstructure:"params"`                                 // Optional type-specific parameters for future extensibility
	WaitBefore  time.Duration  `mapstructure:"wait_before" validate:"omitempty,gte=0"` // Optional wait time before executing the case
	WaitAfter   time.Duration  `mapstructure:"wait_after" validate:"omitempty,gte=0"`  // Optional wait time after executing the case
	Selector    string         `mapstructure:"selector"`                               // YAML path selector for output processing
}

// GetRunPolicy returns the effective run policy, defaulting to "normal" if not set.
func (c *Case) GetRunPolicy() RunPolicy {
	if c.RunPolicy == "" {
		return RunPolicyNormal
	}
	return c.RunPolicy
}

// IsCritical returns true if this case should abort the suite on ERROR.
func (c *Case) IsCritical() bool {
	return c.GetRunPolicy() == RunPolicyCritical
}

// ShouldAlwaysRun returns true if this case should run regardless of prior failures.
func (c *Case) ShouldAlwaysRun() bool {
	return c.GetRunPolicy() == RunPolicyAlways
}

// +schema:inline
// SuiteContent represents the content of a test suite and is only used for schema generation
type SuiteContent struct {
	Description  string   `mapstructure:"description"`
	Cases        []*Case  `mapstructure:"cases" validate:"required,min=1,dive,required"`
	Environments []string `mapstructure:"environments"`
}

type Suite struct {
	Name         string   `mapstructure:"name" validate:"required"`
	Filepath     string   `mapstructure:"filepath"`                                                                 // The path to the file where the suite is defined. Mutually exclusive with all other fields
	Description  string   `mapstructure:"description"`                                                              // Optional description of the test suite purpose
	Cases        []*Case  `mapstructure:"cases" validate:"required_without=Filepath,omitempty,min=1,dive,required"` // Test cases in this suite
	Environments []string `mapstructure:"environments"`                                                             // Required list of environments to run this suite in
}

// DeepCopy creates a deep copy of the Suite.
func (s *Suite) DeepCopy() *Suite {
	newCases := make([]*Case, len(s.Cases))
	for i, c := range s.Cases {
		if c != nil {
			newCase := *c
			newCases[i] = &newCase
		}
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

// IsExternal checks if the suite is defined in an external file
func (s Suite) IsExternal() bool {
	return s.Filepath != ""
}

// Load loads the suite from an external file if applicable
func (s *Suite) Load(configDir string) error {
	if !s.IsExternal() {
		return nil
	}
	originalName := s.Name

	filepath := s.Filepath
	if !path.IsAbs(filepath) {
		filepath = path.Join(configDir, s.Filepath)
	}

	zap.L().Info("Loading external suite", zap.String("file", filepath))
	v := viper.New()
	v.SetConfigFile(filepath)

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read suite file %s: %w", s.Filepath, err)
	}

	zap.L().Info("Suite file loaded", zap.String("file", v.ConfigFileUsed()))

	if err := v.Unmarshal(&s); err != nil {
		return fmt.Errorf("failed to unmarshal suite file %s: %w", s.Filepath, err)
	}

	// Restore name from root config (authoritative)
	s.Name = originalName

	// Clear the filepath after loading
	s.Filepath = ""

	return nil
}

type SnapshotterConfig struct {
	URL    string `mapstructure:"url" validate:"omitempty,url"`
	Binary string `mapstructure:"binary"`
}

type RoverCtlConfig struct {
	DownloadURL string `mapstructure:"download_url" validate:"omitempty,url"`
	Binary      string `mapstructure:"binary" validate:"required"`
}

type Variable struct {
	Name  string `mapstructure:"name" validate:"required"`
	Value string `mapstructure:"value" validate:"required"`
}

type Environments struct {
	Name      string     `mapstructure:"name" validate:"required"`
	Token     string     `mapstructure:"token" validate:"required"`
	Variables []Variable `mapstructure:"variables" validate:"omitempty,dive"`
}

// +schema:inline
type Config struct {
	Snapshotter  SnapshotterConfig `mapstructure:"snapshotter"`
	RoverCtl     RoverCtlConfig    `mapstructure:"roverctl" validate:"required"`
	Environments []Environments    `mapstructure:"environments" validate:"required,min=1,dive"`
	Suites       []Suite           `mapstructure:"suites" validate:"required,min=1,dive"`
	Verbose      bool              `mapstructure:"verbose"`
}

// Validate checks if the config is valid using struct validation tags
func (c *Config) Validate() error {
	return ValidateConfig(c)
}

func (c *Config) LoadSuites(configDir string) error {
	for i := range c.Suites {
		if c.Suites[i].IsExternal() {
			if err := c.Suites[i].Load(configDir); err != nil {
				return err
			}
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
