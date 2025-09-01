// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"encoding/json"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
)

// FormatOutput formats an object according to the specified format
func FormatOutput(obj any, format string) (string, error) {
	switch format {
	case "yaml", "yml":
		bytes, err := yaml.MarshalWithOptions(obj, yaml.Indent(2))
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal to YAML")
		}
		return string(bytes), nil

	case "json":
		bytes, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal to JSON")
		}
		return string(bytes), nil

	default:
		return "", errors.Errorf("unsupported format: %s", format)
	}
}
