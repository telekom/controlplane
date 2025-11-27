// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"encoding/json"
	"github.com/pkg/errors"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime"
)

// RenderMessage renders a text/template with the given template and data in runtime.RawExtension.
// Returns the rendered string or an error.
func renderMessage(template *template.Template, data runtime.RawExtension) (string, error) {

	// Step 1: Unmarshal RawExtension.Raw (JSON bytes) into a map[string]interface{}
	var values map[string]interface{}
	if err := json.Unmarshal(data.Raw, &values); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal RawExtension: %q", data.Raw)
	}

	// Step 1: Execute the template with the unmarshaled data
	var buf bytes.Buffer
	if err := template.Execute(&buf, values); err != nil {
		return "", errors.Wrapf(err, "failed to execute template")
	}

	return buf.String(), nil
}
