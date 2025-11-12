// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
)

// ApplySelector applies a simple YAML path selector to YAML content
// and returns the selected portion
func ApplySelector(content string, selector string) (string, error) {

	path, err := yaml.PathString(selector)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse selector")
	}
	var v any
	if err := path.Read(strings.NewReader(content), &v); err != nil {
		return "", errors.Wrap(err, "failed to apply selector to content. Selector can only be used with YAML content")
	}

	selectedYAML, err := yaml.Marshal(v)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal selected YAML content")
	}
	return string(selectedYAML), nil
}
