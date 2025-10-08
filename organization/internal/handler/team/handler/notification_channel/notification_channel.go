// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package notification_channel

import (
	"context"

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
		return nil
	}

	if _, err := k8sClient.CreateOrUpdate(ctx, channelObj, mutate); err != nil {
		return err
	}

	owner.Status.NotificationChannelRef = types.ObjectRefFromObject(channelObj)

	if err := n.sendNotifications(ctx, owner); err != nil {
		log.Err(err).Msg("failed to send notifications")
	}

	return nil
}

// sendNotifications sends notifications based on team annotations
func (n NotificationChannelHandler) sendNotifications(ctx context.Context, owner *organizationv1.Team) error {
	notificationBuilder := builder.New().
		WithOwner(owner).
		WithSender(notificationv1.SenderTypeSystem, "OrganizationService").
		WithDefaultChannels(ctx, owner.Status.Namespace).
		WithProperties(map[string]any{
			"env":     contextutil.EnvFromContextOrDie(ctx),
			"team":    owner.Spec.Name,
			"group":   owner.Spec.Group,
			"members": owner.Spec.Members,
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

	notification, err = notificationBuilder.WithPurpose("team members changed").
		WithNameSuffix(hash.ComputeHash(owner.Spec.Members, nil)).
		Build(ctx)
	if err != nil {
		return err
	}

	if owner.GetGeneration() > 1 {
		if owner.Status.NotificationMemberChangedRef == nil {
			notification, err = notificationBuilder.Send(ctx)
			if err != nil {
				return err
			}
			owner.Status.NotificationMemberChangedRef = types.ObjectRefFromObject(notification)
		} else if owner.Status.NotificationMemberChangedRef.Name != notification.Name {
			notification, err = notificationBuilder.Send(ctx)
			if err != nil {
				return err
			}
			owner.Status.NotificationMemberChangedRef = types.ObjectRefFromObject(notification)
		}
	}

	return nil
}

func rotateTokenNotification(ctx context.Context, owner *organizationv1.Team, notificationBuilder builder.NotificationBuilder) error {
	var notification *notificationv1.Notification
	var err error

	notification, err = notificationBuilder.WithPurpose("token rotated").
		WithNameSuffix(hash.ComputeHash(owner.Status.TeamToken, nil)).
		Build(ctx)
	if err != nil {
		return err
	}

	if owner.Status.NotificationTokenRotateRef == nil {
		notification, err = notificationBuilder.Send(ctx)
		if err != nil {
			return err
		}
	} else if owner.Status.NotificationTokenRotateRef.Name != notification.Name {
		notification, err = notificationBuilder.Send(ctx)
		if err != nil {
			return err
		}
	}

	owner.Status.NotificationTokenRotateRef = types.ObjectRefFromObject(notification)
	return nil
}

func onboardingNotification(ctx context.Context, owner *organizationv1.Team, notificationBuilder builder.NotificationBuilder) error {
	if owner.GetGeneration() == 1 {
		notification, err := notificationBuilder.WithPurpose("onboarded").Send(ctx)
		if err != nil {
			return err
		}
		owner.Status.NotificationOnboardingRef = types.ObjectRefFromObject(notification)
	}
	return nil
}

func (n NotificationChannelHandler) Identifier() string {
	return "notification-channel"
}
