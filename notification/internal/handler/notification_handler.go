// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/internal/sender"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

var _ handler.Handler[*notificationv1.Notification] = &NotificationHandler{}

type NotificationHandler struct {
	NotificationSender sender.NotificationSender
}

func (n *NotificationHandler) CreateOrUpdate(ctx context.Context, notification *notificationv1.Notification) error {

	var shouldBlock = false

	// lets go channel by channel
	for _, channelRef := range notification.Spec.Channels {

		channelKey := channelToMapKey(channelRef)
		// first lets check if the notification was already successfully sent
		if alreadySent(channelKey, notification) {
			continue
		}

		// get the channel object
		channel, err := getChannelByRef(ctx, channelRef)
		if err != nil {
			shouldBlock = true
			addResultToStatus(notification, channelKey, false, err.Error())
			continue
		}

		// resolve the template
		template, err := resolveTemplate(ctx, channel, notification.Spec.Purpose)
		if err != nil {
			shouldBlock = true
			addResultToStatus(notification, channelKey, false, err.Error())
			continue
		}

		// check template placeholders vs schema
		// todo later

		// render
		renderedSubject, err := renderMessage(template.Spec.SubjectTemplate, notification.Spec.Properties)
		if err != nil {
			addResultToStatus(notification, channelKey, false, err.Error())
			continue
		}

		renderedBody, err := renderMessage(template.Spec.Template, notification.Spec.Properties)
		if err != nil {
			addResultToStatus(notification, channelKey, false, err.Error())
			continue
		}

		// better pass to sender service
		err = n.NotificationSender.ProcessNotification(ctx, channel, renderedSubject, renderedBody)
		if err != nil {
			addResultToStatus(notification, channelKey, false, err.Error())
			continue
		}

		addResultToStatus(notification, channelKey, true, "Successfully sent")
	}

	if shouldBlock {
		notification.SetCondition(condition.NewBlockedCondition("Channel or template cannot be resolved"))
		notification.SetCondition(condition.NewNotReadyCondition("NotificationSendingFailed", "Some notifications were not sent"))
	} else {
		if hasFailedSendAttempt(notification.Status.States) {
			notification.SetCondition(condition.NewProcessingCondition("Retrying", "Retrying failed notifications"))
			notification.SetCondition(condition.NewNotReadyCondition("Retrying", "Some notifications were not sent"))
		} else {
			notification.SetCondition(condition.NewReadyCondition("Provisioned", "Notification is provisioned"))
			notification.SetCondition(condition.NewDoneProcessingCondition("Notification is done processing"))
		}
	}

	return nil
}

func alreadySent(key string, notification *notificationv1.Notification) bool {
	if notification.Status.States == nil || len(notification.Status.States) == 0 {
		return false
	}

	if state, found := notification.Status.States[key]; found {
		return state.Sent
	}
	return false
}

func hasFailedSendAttempt(statesMap map[string]notificationv1.SendState) bool {
	for _, state := range statesMap {
		if !state.Sent {
			return true
		}
	}
	return false
}

func addResultToStatus(notification *notificationv1.Notification, channelId string, success bool, message string) {
	if notification.Status.States == nil {
		notification.Status.States = make(map[string]notificationv1.SendState)
	}

	notification.Status.States[channelId] = notificationv1.SendState{
		Timestamp:    metav1.Now(),
		Sent:         success,
		ErrorMessage: message,
	}
}

func resolveTemplate(ctx context.Context, channel *notificationv1.NotificationChannel, purpose string) (*notificationv1.NotificationTemplate, error) {
	// channel name - channel--<teamname>--<type> - example: channel--eni--hyperion--mail
	// template name - template--<purpose>--<type> - example: template--api-subscription-approved--chat

	scopedClient := client.ClientFromContextOrDie(ctx)

	templateRef := types.ObjectRef{
		Name:      buildTemplateName(channel, purpose),
		Namespace: contextutil.EnvFromContextOrDie(ctx),
	}

	template := &notificationv1.NotificationTemplate{}

	err := scopedClient.Get(ctx, templateRef.K8s(), template)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get template %q", templateRef)
	}

	if !meta.IsStatusConditionTrue(template.GetConditions(), condition.ConditionTypeReady) {
		return nil, errors.New(fmt.Sprintf("Template %q found but its not ready", types.ObjectRefFromObject(template)))
	}

	return template, nil
}

func channelToMapKey(channel types.ObjectRef) string {
	return fmt.Sprintf("%s/%s", channel.Namespace, channel.Name)
}

func buildTemplateName(channel *notificationv1.NotificationChannel, purpose string) string {
	// channel name - channel--<teamname>--<type> - example: channel--eni--hyperion--mail
	// template name - template--<purpose>--<type> - example: template--api-subscription-approved--chat
	return fmt.Sprintf("template--%s--%s", purpose, strings.ToLower(string(channel.NotificationType())))
}

func getChannelByRef(ctx context.Context, ref types.ObjectRef) (*notificationv1.NotificationChannel, error) {
	scopedClient := client.ClientFromContextOrDie(ctx)

	channel := notificationv1.NotificationChannel{}
	err := scopedClient.Get(ctx, ref.K8s(), &channel)
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting channel %q", ref)
	}

	if err = condition.EnsureReady(&channel); err != nil {
		return nil, errors.Wrapf(err, "Channel %q found but its not ready", ref)
	}
	return &channel, nil

}

func (n *NotificationHandler) Delete(ctx context.Context, notification *notificationv1.Notification) error {
	return nil
}
