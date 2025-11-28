// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/notification/internal/rendering"
	"github.com/telekom/controlplane/notification/internal/templatecache"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ handler.Handler[*notificationv1.NotificationTemplate] = &NotificationTemplateHandler{}

type NotificationTemplateHandler struct {
	Cache *templatecache.TemplateCache
}

func (n *NotificationTemplateHandler) CreateOrUpdate(ctx context.Context, template *notificationv1.NotificationTemplate) error {
	// Validate template content based on channel type
	if err := n.validateTemplate(template); err != nil {
		template.SetCondition(condition.NewReadyCondition("ValidationFailed", err.Error()))
		return err
	}

	// parse them in advance - save repeated operation for each notification
	parsedTemplates, err := rendering.ParseTemplate(template)
	if err != nil {
		return ctrlerrors.BlockedErrorf("Parsing of template failed with error %v", err.Error())
	}

	// cache the parsed templates
	n.Cache.Set(template.Name, parsedTemplates)

	template.SetCondition(condition.NewReadyCondition("Provisioned", "Notification template is provisioned"))
	template.SetCondition(condition.NewDoneProcessingCondition("Notification template is done processing"))
	return nil
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
