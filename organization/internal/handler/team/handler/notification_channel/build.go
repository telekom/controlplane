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
	return &notificationv1.NotificationChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Status.Namespace,
		},

		Spec: notificationv1.NotificationChannelSpec{
			Email: &notificationv1.EmailConfig{
				Recipients:     []notificationv1.EmailString{notificationv1.EmailString(owner.Spec.Email)},
				CCRecipients:   nil,
				SMTPHost:       "example.com",
				SMTPPort:       443,
				From:           "telekom@example.com",
				Authentication: nil,
			}, //todo
			MsTeams: nil, //todo
			Webhook: nil, //todo
			Ignore:  nil, //todo
		},
	}
}
