// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"encoding/json"
	sprig "github.com/go-task/slim-sprig/v3"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/notification/internal/templatecache"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"

	texttemplate "text/template"
)

var _ handler.Handler[*notificationv1.NotificationTemplate] = &NotificationTemplateHandler{}

type NotificationTemplateHandler struct {
	Cache           *templatecache.TemplateCache
	CustomFunctions texttemplate.FuncMap
}

func (n *NotificationTemplateHandler) CreateOrUpdate(ctx context.Context, template *notificationv1.NotificationTemplate) error {
	// Validate template content based on channel type
	if err := n.validateTemplate(template); err != nil {
		template.SetCondition(condition.NewReadyCondition("ValidationFailed", err.Error()))
		return err
	}

	// parse them in advance - save repeated operation for each notification
	parsedTemplates, err := parseTemplate(template, n.CustomFunctions)
	if err != nil {
		template.SetCondition(condition.NewReadyCondition("ParsingFailed", err.Error()))
	}

	// cache the parsed templates
	n.Cache.Set(template.Name, parsedTemplates)

	template.SetCondition(condition.NewReadyCondition("Provisioned", "Notification template is provisioned"))
	template.SetCondition(condition.NewDoneProcessingCondition("Notification template is done processing"))
	return nil
}

// parseTemplate parses both the subject and body templates, returns an error if the attempt fails
func parseTemplate(template *notificationv1.NotificationTemplate, customFunctions texttemplate.FuncMap) (*templatecache.TemplateWrapper, error) {

	// merge sprig + custom funcs
	funcs := sprig.FuncMap()
	for k, v := range customFunctions {
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

// validateTemplate validates the template content based on channel type
func (n *NotificationTemplateHandler) validateTemplate(template *notificationv1.NotificationTemplate) error {
	switch template.Spec.ChannelType {
	case "MsTeams":
		// MS Teams templates must be valid JSON (Adaptive Cards or MessageCard format)
		if !json.Valid([]byte(template.Spec.Template)) {
			return errors.New("invalid JSON template for MsTeams channel: template must be valid JSON")
		}
	case "Email":
		// Email templates can be plain text or HTML, no strict validation needed
	case "Webhook":
		// Webhook templates are flexible, typically JSON but not strictly required
		// Optionally validate if it looks like JSON
	}

	// Validate schema if provided
	if len(template.Spec.Schema.Raw) > 0 {
		if !json.Valid(template.Spec.Schema.Raw) {
			return errors.New("invalid JSON schema: schema must be valid JSON")
		}
	}

	return nil
}

func (n *NotificationTemplateHandler) Delete(ctx context.Context, template *notificationv1.NotificationTemplate) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Deleting template from cache", "name", template.Name)

	n.Cache.Delete(template.Name)
	return nil
}
