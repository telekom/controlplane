// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rendering

import (
	"bytes"
	sprig "github.com/go-task/slim-sprig/v3"
	"github.com/pkg/errors"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/internal/templatecache"
	"k8s.io/apimachinery/pkg/runtime"
	texttemplate "text/template"

	"encoding/json"
)

// parseTemplate parses both the subject and body templates, returns an error if the attempt fails
func ParseTemplate(template *notificationv1.NotificationTemplate) (*templatecache.TemplateWrapper, error) {

	// merge sprig + custom funcs
	funcs := sprig.FuncMap()
	for k, v := range getCustomTemplateFunctions() {
		funcs[k] = v
	}

	parsedSubjectTemplate, err := texttemplate.New(template.Name + "--subject").Funcs(funcs).Parse(template.Spec.SubjectTemplate)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse subject template")
	}

	parsedBodyTemplate, err := texttemplate.New(template.Name + "--body").Funcs(funcs).Parse(template.Spec.Template)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse body template")
	}

	return &templatecache.TemplateWrapper{
		BodyTemplate:    parsedBodyTemplate,
		SubjectTemplate: parsedSubjectTemplate,
	}, nil
}

// RenderMessage renders a text/template with the given template and data in runtime.RawExtension.
// Returns the rendered string or an error.
func RenderMessage(template *texttemplate.Template, data runtime.RawExtension) (string, error) {

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

// place custom functions here
// see template_renderer_test for a simple example
func getCustomTemplateFunctions() texttemplate.FuncMap {
	return texttemplate.FuncMap{}
}
