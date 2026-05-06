// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())

	// Register a tag name function to use mapstructure tags for field names
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("mapstructure"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	// Register custom field-level validators - panic on failure since this is initialization
	if err := validate.RegisterValidation("run_policy", validateRunPolicy); err != nil {
		panic(fmt.Sprintf("failed to register run_policy validator: %v", err))
	}

	// Register struct-level validators
	validate.RegisterStructValidation(validateSnapshotterConfig, SnapshotterConfig{})
	validate.RegisterStructValidation(validateConfigStructureLevel, Config{})
	validate.RegisterStructValidation(validateSuite, Suite{})
}

// ValidateConfig validates the configuration using struct tags and custom validators.
// It returns a user-friendly error message aggregating all validation failures,
// or nil if the configuration is valid.
func ValidateConfig(c *Config) error {
	err := validate.Struct(c)
	if err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			return formatValidationErrors(validationErrors)
		}
		return fmt.Errorf("validation error: %w", err)
	}
	return nil
}

// validateRunPolicy validates RunPolicy enum values.
func validateRunPolicy(fl validator.FieldLevel) bool {
	policy := RunPolicy(fl.Field().String())
	if policy == "" {
		return true // Empty defaults to "RunOnSuccess" at runtime
	}
	return policy.IsValid()
}

// validateSnapshotterConfig ensures at least URL or Binary is provided.
func validateSnapshotterConfig(sl validator.StructLevel) {
	cfg, ok := sl.Current().Interface().(SnapshotterConfig)
	if !ok {
		return // Shouldn't happen, but be defensive
	}
	if cfg.URL == "" && cfg.Binary == "" {
		sl.ReportError(cfg.URL, "Snapshotter", "snapshotter", "url_or_binary_required", "")
	}
}

// validateConfigStructureLevel performs all Config-level validations including
// cross-reference checks and uniqueness constraints.
func validateConfigStructureLevel(sl validator.StructLevel) {
	cfg, ok := sl.Current().Interface().(Config)
	if !ok {
		return // Shouldn't happen, but be defensive
	}

	// Validate environment cross-references
	validEnvs := make(map[string]bool)
	for _, env := range cfg.Environments {
		validEnvs[env.Name] = true
	}

	// Check suite-level environment references
	for i, suite := range cfg.Suites {
		for j, envName := range suite.Environments {
			if envName != "" && !validEnvs[envName] {
				sl.ReportError(
					cfg.Suites[i].Environments[j],
					fmt.Sprintf("Suites[%d].Environments[%d]", i, j),
					"environments",
					"env_reference",
					envName,
				)
			}
		}

		// Check case-level environment overrides
		for j, c := range suite.Cases {
			if c != nil && c.Environment != "" && !validEnvs[c.Environment] {
				sl.ReportError(
					c.Environment,
					fmt.Sprintf("Suites[%d].Cases[%d].Environment", i, j),
					"environment",
					"env_reference",
					c.Environment,
				)
			}
		}
	}

	// Check for duplicate environment names
	envNames := make(map[string]bool)
	for i, env := range cfg.Environments {
		if env.Name != "" {
			if envNames[env.Name] {
				sl.ReportError(
					cfg.Environments[i].Name,
					fmt.Sprintf("Environments[%d].Name", i),
					"name",
					"unique_env_name",
					env.Name,
				)
			}
			envNames[env.Name] = true
		}
	}

	// Check for duplicate suite names
	suiteNames := make(map[string]bool)
	for i, suite := range cfg.Suites {
		if suite.Name != "" {
			if suiteNames[suite.Name] {
				sl.ReportError(
					cfg.Suites[i].Name,
					fmt.Sprintf("Suites[%d].Name", i),
					"name",
					"unique_suite_name",
					suite.Name,
				)
			}
			suiteNames[suite.Name] = true
		}
	}
}

func validateSuite(sl validator.StructLevel) {
	suite, ok := sl.Current().Interface().(Suite)
	if !ok {
		return
	}

	hasFilepath := suite.Filepath != ""
	hasCases := len(suite.Cases) > 0

	if hasFilepath && hasCases {
		sl.ReportError(suite.Filepath, "filepath", "Filepath",
			"filepath_cases_exclusive", "")
	}

	if !hasFilepath && !hasCases {
		sl.ReportError(suite.Cases, "cases", "Cases",
			"min", "1")
	}
}

// formatValidationErrors converts validator.ValidationErrors to user-friendly messages
func formatValidationErrors(errs validator.ValidationErrors) error {
	var messages []string
	for _, e := range errs {
		messages = append(messages, formatFieldError(e))
	}
	return fmt.Errorf("configuration validation failed:\n  - %s",
		strings.Join(messages, "\n  - "))
}

func formatFieldError(e validator.FieldError) string {
	field := e.Namespace()
	// Remove "Config." prefix for cleaner output
	field = strings.TrimPrefix(field, "Config.")

	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "min":
		return fmt.Sprintf("%s must have at least %s item(s)", field, e.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s (got: '%v')", field, e.Param(), e.Value())
	case "unique":
		return fmt.Sprintf("%s must have unique '%s' values", field, e.Param())
	case "run_policy":
		validValues := make([]string, len(ValidRunPolicies))
		for i, p := range ValidRunPolicies {
			validValues[i] = string(p)
		}
		return fmt.Sprintf("%s must be one of: %s (got: '%v')", field, strings.Join(validValues, ", "), e.Value())
	case "url_or_binary_required":
		return "Snapshotter: either 'url' or 'binary' must be specified"
	case "url":
		return fmt.Sprintf("%s must be a valid URL (got: '%v')", field, e.Value())
	case "env_reference":
		return fmt.Sprintf("%s references unknown environment '%s'", field, e.Param())
	case "unique_env_name":
		return fmt.Sprintf("Environments must have unique names (duplicate: '%s')", e.Param())
	case "unique_suite_name":
		return fmt.Sprintf("Suites must have unique names (duplicate: '%s')", e.Param())
	case "gte":
		return fmt.Sprintf("%s must be >= %s", field, e.Param())
	case "filepath_cases_exclusive":
		return fmt.Sprintf("%s: 'filepath' and 'cases' are mutually exclusive", field)
	default:
		return fmt.Sprintf("%s: validation failed on '%s'", field, e.Tag())
	}
}
