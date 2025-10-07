// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime"
)

// RenderMessage renders a text/template with the given template string and data in runtime.RawExtension.
// Returns the rendered string or an error.
func renderMessage(tmplStr string, data runtime.RawExtension) (string, error) {
	// Step 1: Unmarshal RawExtension.Raw (JSON bytes) into a map[string]interface{}
	var values map[string]interface{}
	if err := json.Unmarshal(data.Raw, &values); err != nil {
		return "", fmt.Errorf("failed to unmarshal RawExtension: %w", err)
	}

	// Step 2: Parse the template string
	tmpl, err := template.New("message").Parse(tmplStr)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse template")
	}

	// Step 3: Execute the template with the unmarshaled data
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
