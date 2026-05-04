// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rendering

import (
	"bytes"
	"encoding/json"
	"fmt"
	texttemplate "text/template"
	"time"

	sprig "github.com/go-task/slim-sprig/v3"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/internal/templatecache"
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
		return nil, fmt.Errorf("failed to parse subject template: %w", err)
	}

	parsedBodyTemplate, err := texttemplate.New(template.Name + "--body").Funcs(funcs).Parse(template.Spec.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse body template: %w", err)
	}

	var parsedAttachments []templatecache.ParsedAttachment
	for i, att := range template.Spec.Attachments {
		parsed, err := texttemplate.New(fmt.Sprintf("%s--attachment-%d", template.Name, i)).Funcs(funcs).Parse(att.Template)
		if err != nil {
			return nil, fmt.Errorf("failed to parse attachment template %q: %w", att.Filename, err)
		}
		parsedAttachments = append(parsedAttachments, templatecache.ParsedAttachment{
			Filename:        att.Filename,
			ContentType:     att.ContentType,
			ContentTemplate: parsed,
		})
	}

	return &templatecache.TemplateWrapper{
		BodyTemplate:    parsedBodyTemplate,
		SubjectTemplate: parsedSubjectTemplate,
		Attachments:     parsedAttachments,
	}, nil
}

// UnmarshalProperties converts a JSON RawExtension into a template data map.
func UnmarshalProperties(raw json.RawMessage) (map[string]interface{}, error) {
	var values map[string]interface{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
	}
	return values, nil
}

// RenderMessage renders a text/template against pre-parsed template data.
// Returns the rendered string or an error.
func RenderMessage(template *texttemplate.Template, values map[string]interface{}) (string, error) {
	var buf bytes.Buffer
	if err := template.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.String(), nil
}

// place custom functions here
// see template_renderer_test for a simple example
func getCustomTemplateFunctions() texttemplate.FuncMap {
	return texttemplate.FuncMap{
		// icsTime converts an RFC 3339 timestamp to iCalendar format (e.g. "20060102T150405Z").
		"icsTime": func(input string) (string, error) {
			t, err := time.Parse(time.RFC3339, input)
			if err != nil {
				return "", fmt.Errorf("icsTime: invalid RFC3339 time %q: %w", input, err)
			}
			return t.UTC().Format("20060102T150405Z"), nil
		},
	}
}

// RenderedAttachment holds a fully rendered attachment ready for sending.
type RenderedAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// RenderAttachments renders all attachment templates against pre-parsed template data.
func RenderAttachments(attachments []templatecache.ParsedAttachment, values map[string]interface{}) ([]RenderedAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}

	var result []RenderedAttachment
	for _, att := range attachments {
		var buf bytes.Buffer
		if err := att.ContentTemplate.Execute(&buf, values); err != nil {
			return nil, fmt.Errorf("failed to render attachment %q: %w", att.Filename, err)
		}
		result = append(result, RenderedAttachment{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Content:     buf.Bytes(),
		})
	}
	return result, nil
}
