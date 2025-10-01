// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package data

import notificationv1 "github.com/telekom/controlplane/notification/api/v1"

type NotificationType string

const (
	NotificationTypeMail     NotificationType = "mail"
	NotificationTypeChat     NotificationType = "chat"
	NotificationTypeCallback NotificationType = "callback"
)

type NotificationData interface {
	NotificationType() NotificationType
}

type NotificationDataBody struct {
	Title string
	Body  string
}

var _ NotificationData = MailNotificationData{}

type MailNotificationData struct {
	Config notificationv1.MailConfig
	NotificationDataBody
}

func (MailNotificationData) NotificationType() NotificationType {
	return NotificationTypeMail
}

type ChatNotificationData struct {
	Config notificationv1.ChatConfig
	NotificationDataBody
}

func (ChatNotificationData) NotificationType() NotificationType {
	return NotificationTypeChat
}

type CallbackNotificationData struct {
	Config notificationv1.CallbackConfig
	NotificationDataBody
}

func (CallbackNotificationData) NotificationType() NotificationType {
	return NotificationTypeCallback
}
