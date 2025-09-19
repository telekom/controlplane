// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"time"

	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

// NotificationBuilder provides a fluent API for creating and sending notifications.
// It abstracts the complexity of notification creation, allowing external domains
// to easily send notifications without understanding the internal details of the
// notification system.
//
// Usage example:
//
//	notification, err := builder.NewNotificationBuilder().
//		WithNamespace("default").
//		WithPurpose("ApprovalGranted").
//		WithSystemSender("ApprovalSystem").
//		WithChannels("team-channel").
//		WithProperties(map[string]any{
//			"resourceName": "example-resource",
//			"approvedBy": "admin",
//		}).
//		Send(ctx)
type NotificationBuilder interface {

	// WithNamespace sets the namespace of the notification.
	// This is a required field and must be set before building the object.
	// It is recommended to use the namespace of the resource that is triggering the notification.
	WithNamespace(namespace string) NotificationBuilder

	// WithPurpose sets the purpose of the notification.
	// This is a required field and must be set before building the object.
	// It is used to select the notification template.
	WithPurpose(purpose string) NotificationBuilder

	// WithSender sets the sender of the notification.
	// This is an optional field. If not set, the sender will be set to "System".
	WithSender(senderType notificationv1.SenderType, senderName string) NotificationBuilder

	// WithDefaultChannel will set all available channels for the given namespace
	WithDefaultChannels(ctx context.Context, namespace string) NotificationBuilder
	// WithChannels sets the channels to send the notification to.
	WithChannels(channels ...string) NotificationBuilder

	// WithProperties will add the given properties to the notification
	// Properties are used to render the notification template
	// The properties must not exceed 1024 bytes when serialized to JSON
	WithProperties(properties map[string]any) NotificationBuilder

	// Build builds the Notification object
	Build() (*notificationv1.Notification, error)

	// Send will send the notification asynchronously
	Send(ctx context.Context) (*notificationv1.Notification, error)

	// SendAndWait will send the notification and wait for it to be processed or timeout
	SendAndWait(ctx context.Context, timeout time.Duration) (*notificationv1.Notification, error)
}

type notificationBuilder struct{}
