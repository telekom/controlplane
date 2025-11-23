// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package notification_channel

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/hash"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
)

const separator = "--"

type NotificationChannelHandler struct {
}

var _ handler.ObjectHandler = &NotificationChannelHandler{}

func (n NotificationChannelHandler) Delete(ctx context.Context, owner *organizationv1.Team) error {
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	channelObj := buildNotificationChannelObj(owner)
	return k8sClient.Delete(ctx, channelObj)
}

func (n NotificationChannelHandler) CreateOrUpdate(ctx context.Context, owner *organizationv1.Team) error {
	log := log.Ctx(ctx)
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	channelObj := buildNotificationChannelObj(owner)

	mutate := func() error {
		channelObj.SetLabels(owner.GetLabels())

		recipientsMails := make([]string, 0)
		for _, member := range owner.Spec.Members {
			recipientsMails = append(recipientsMails, member.Email)
		}
		recipientsMails = append(recipientsMails, owner.Spec.Email)

		channelObj.Spec = notificationv1.NotificationChannelSpec{
			Email: &notificationv1.EmailConfig{
				Recipients: recipientsMails,
			},
			// TODO: At a later stage, teams can configure how to receive notifications. For now, only mail
			MsTeams: nil,
			Webhook: nil,
			Ignore:  nil,
		}

		return nil
	}

	if _, err := k8sClient.CreateOrUpdate(ctx, channelObj, mutate); err != nil {
		return err
	}

	owner.Status.NotificationChannelRef = types.ObjectRefFromObject(channelObj)
	if owner.Status.NotificationsRef == nil {
		owner.Status.NotificationsRef = make(map[string]*types.ObjectRef)
	}
	if err := n.sendNotifications(ctx, owner); err != nil {
		log.Err(err).Msg("failed to send notifications")
	}

	return nil
}

// sendNotifications sends notifications based on team
func (n NotificationChannelHandler) sendNotifications(ctx context.Context, owner *organizationv1.Team) error {
	notificationBuilder := builder.New().
		WithNamespace(owner.Status.Namespace).
		WithSender(notificationv1.SenderTypeSystem, "OrganizationService").
		WithDefaultChannels(ctx, owner.Status.Namespace).
		WithProperties(map[string]any{
			"environment": contextutil.EnvFromContextOrDie(ctx),
			"team":        owner.Spec.Name,
			"group":       owner.Spec.Group,
			"members":     owner.Spec.Members,
		})

	err := onboardingNotification(ctx, owner, notificationBuilder)
	if err != nil {
		return err
	}

	err = rotateTokenNotification(ctx, owner, notificationBuilder)
	if err != nil {
		return err
	}

	err = memberChangedNotification(ctx, owner, notificationBuilder)
	if err != nil {
		return err
	}

	return nil
}

func memberChangedNotification(ctx context.Context, owner *organizationv1.Team, notificationBuilder builder.NotificationBuilder) error {
	var notification *notificationv1.Notification
	var err error

	notification, err = notificationBuilder.WithPurpose("team-members-changed").
		WithName("team-members-changed--" + hash.ComputeHash(owner.Spec.Members, nil)).
		Build(ctx)
	if err != nil {
		return err
	}

	existingNotificationRef, ok := owner.Status.NotificationsRef["team-members-changed"]

	sendNotification := true
	if ok {
		if notification.GetName() == existingNotificationRef.GetName() {
			sendNotification = false
		}
	}

	if owner.GetGeneration() > 1 && sendNotification {
		notification, err = notificationBuilder.Send(ctx)
		if err != nil {
			return err
		}
		owner.Status.NotificationsRef["team-members-changed"] = types.ObjectRefFromObject(notification)

	}

	return nil
}

func rotateTokenNotification(ctx context.Context, owner *organizationv1.Team, notificationBuilder builder.NotificationBuilder) error {
	var notification *notificationv1.Notification
	var err error

	tokenHash := hash.ComputeHash(owner.GetTeamToken(), nil)
	notificationName := "token-rotated--" + tokenHash

	notification, err = notificationBuilder.WithPurpose("token-rotated").
		WithName(notificationName).
		Build(ctx)
	if err != nil {
		return err
	}

	createNotification := true
	existingNotificationRef, ok := owner.Status.NotificationsRef["token-rotated"]
	if ok {
		createNotification = hasTeamTokenChanged(existingNotificationRef.GetName(), notification.GetName())
	}

	if createNotification {
		notification, err = notificationBuilder.Send(ctx)
		if err != nil {
			return err
		}
	}

	owner.Status.NotificationsRef["token-rotated"] = types.ObjectRefFromObject(notification)

	return nil
}

func onboardingNotification(ctx context.Context, owner *organizationv1.Team, notificationBuilder builder.NotificationBuilder) error {
	if _, ok := owner.Status.NotificationsRef["onboarded"]; owner.GetGeneration() == 1 && !ok {
		notification, err := notificationBuilder.WithPurpose("onboarded").
			WithName("onboarded").
			Send(ctx)
		if err != nil {
			return err
		}
		owner.Status.NotificationsRef["onboarded"] = types.ObjectRefFromObject(notification)
	}
	return nil
}

func (n NotificationChannelHandler) Identifier() string {
	return "notification-channel"
}

func hasTeamTokenChanged(old string, new string) bool {
	// notification name is token-rotated--<tokenHash>--<specHash>

	// split by delimiter
	oldParts := strings.Split(old, separator)
	if len(oldParts) < 2 {
		return false
	}

	newParts := strings.Split(new, separator)
	if len(newParts) < 2 {
		return false
	}

	return oldParts[1] != newParts[1]
}
