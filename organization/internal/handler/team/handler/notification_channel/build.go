// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package notification_channel

import (
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	organisationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/handler/team/handler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildNotificationChannelObj(owner *organisationv1.Team) *notificationv1.NotificationChannel {
	name := owner.Spec.Group + handler.Separator + owner.Spec.Name

	memberMails := make([]string, 0)
	for _, member := range owner.Spec.Members {
		memberMails = append(memberMails, member.Email)
	}

	return &notificationv1.NotificationChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Status.Namespace,
		},

		Spec: notificationv1.NotificationChannelSpec{
			Email: &notificationv1.EmailConfig{
				Recipients:   []string{owner.Spec.Email},
				CCRecipients: memberMails,
				From:         "telekom@example.com",
			},
			// TODO: At a later stage, teams can configure how to receive notifications. For now, only mail
			MsTeams: nil,
			Webhook: nil,
			Ignore:  nil,
		},
	}
}
