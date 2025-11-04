// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"encoding/json"
	"github.com/go-logr/logr"
	"maps"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/pkg/errors"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notificationchannels,verbs=get;list;watch

// NotificationBuilder provides a fluent API for creating and sending notifications.
// It abstracts the complexity of notification creation, allowing external domains
// to easily send notifications without understanding the internal details of the
// notification system.
//
// Usage example:
//
//	notification, err := builder.NewNotificationBuilder().
//		WithOwner(eventSourceResource). // eventSourceResource should be the Kubernetes object that triggered the notification
//		WithPurpose("ApprovalGranted").
//		WithSystemSender("ApprovalSystem").
//		WithChannels(types.ObjectRef{Name: "email", Namespace: "env--group--team"}).
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

	// WithOwner sets the owner of the notification.
	// This is an optional field. If set, the notification will have an owner reference to the given object.
	// It is recommended to set the owner to ensure proper garbage collection of the notification.
	// If the namespace is not set, it will be set to the namespace of the owner.
	WithOwner(owner client.Object) NotificationBuilder

	// WithPurpose sets the purpose of the notification.
	// This is a required field and must be set before building the object.
	// It is used to select the notification template.
	WithPurpose(purpose string) NotificationBuilder

	// WithSender sets the sender of the notification.
	// This is an optional field. If not set, the sender will be set to "System".
	WithSender(senderType notificationv1.SenderType, senderName string) NotificationBuilder

	// WithName sets the name of the notification. If omitted empty, the name will be generated based on the purpose.
	// A hash will be appended at the end by the builder.
	WithName(nameSuffix string) NotificationBuilder

	// WithDefaultChannel will set all available channels for the given namespace
	WithDefaultChannels(ctx context.Context, namespace string) NotificationBuilder
	// WithChannels sets the channels to send the notification to.
	WithChannels(channels ...types.ObjectRef) NotificationBuilder

	// WithProperties will add the given properties to the notification
	// Properties are used to render the notification template
	// The properties must not exceed 1024 bytes when serialized to JSON
	WithProperties(properties map[string]any) NotificationBuilder

	// Build builds the Notification object
	Build(ctx context.Context) (*notificationv1.Notification, error)

	// Send will send the notification asynchronously
	Send(ctx context.Context) (*notificationv1.Notification, error)
}

var _ NotificationBuilder = &notificationBuilder{}

// notificationBuilder implements the NotificationBuilder interface
type notificationBuilder struct {
	// The notification object being built
	Notification *notificationv1.Notification

	Owner client.Object

	Name string

	Errors []error
}

// New creates a new NotificationBuilder
func New() NotificationBuilder {
	return &notificationBuilder{
		Notification: &notificationv1.Notification{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: notificationv1.NotificationSpec{},
		},
	}
}

// WithChannels implements NotificationBuilder.
func (n *notificationBuilder) WithChannels(channels ...types.ObjectRef) NotificationBuilder {
	n.Notification.Spec.Channels = append(n.Notification.Spec.Channels, channels...)
	return n
}

func (n *notificationBuilder) WithName(name string) NotificationBuilder {
	n.Name = name
	return n
}

func (n *notificationBuilder) WithDefaultChannels(ctx context.Context, namespace string) NotificationBuilder {
	log := logr.FromContextOrDiscard(ctx)
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	channelList := &notificationv1.NotificationChannelList{}
	err := k8sClient.List(ctx, channelList, client.InNamespace(namespace))
	if err != nil {
		n.Errors = append(n.Errors, errors.Wrap(err, "failed to list notification channels"))
		return n
	}

	log.V(1).Info("Found channels in namespace", "namespace", namespace, "count", len(channelList.Items), "channels", channelList.Items)

	// todo - remove this after demo - there might be a bug when channels are not yet ready so the first notification doesnt see them, let add a short wait
	if len(channelList.Items) == 0 {
		log.V(1).Info("Waiting for 2 seconds...")
		time.Sleep(2 * time.Second) // pauses execution for 2 seconds
		channelList = &notificationv1.NotificationChannelList{}
		err = k8sClient.List(ctx, channelList, client.InNamespace(namespace))
		if err != nil {
			n.Errors = append(n.Errors, errors.Wrap(err, "failed to list notification channels"))
			return n
		}
		log.V(1).Info("AFTER WAITING - Found channels in namespace", "namespace", namespace, "count", len(channelList.Items), "channels", channelList.Items)
	}

	for _, channel := range channelList.Items {
		n.Notification.Spec.Channels = append(n.Notification.Spec.Channels, *types.ObjectRefFromObject(&channel))
	}
	return n
}

func (n *notificationBuilder) WithNamespace(namespace string) NotificationBuilder {
	if n.Notification.Namespace != "" {
		return n
	}
	n.Notification.Namespace = namespace
	return n
}

func (n *notificationBuilder) WithOwner(owner client.Object) NotificationBuilder {
	if owner == nil {
		return n
	}
	n.Owner = owner
	n.Notification.Namespace = owner.GetNamespace()
	maps.Copy(n.Notification.Labels, owner.GetLabels())
	maps.Copy(n.Notification.Annotations, owner.GetAnnotations())

	return n
}

func (n *notificationBuilder) WithProperties(properties map[string]any) NotificationBuilder {
	b, err := json.Marshal(properties)
	if err != nil {
		n.Errors = append(n.Errors, errors.Wrap(err, "failed to marshal properties"))
		return n
	}
	n.Notification.Spec.Properties = runtime.RawExtension{Raw: b}
	return n
}

func (n *notificationBuilder) WithPurpose(purpose string) NotificationBuilder {
	n.Notification.Spec.Purpose = purpose
	return n
}

func (n *notificationBuilder) WithSender(senderType notificationv1.SenderType, senderName string) NotificationBuilder {
	n.Notification.Spec.Sender = notificationv1.Sender{
		Type: senderType,
		Name: senderName,
	}
	return n
}

func (n *notificationBuilder) Build(ctx context.Context) (*notificationv1.Notification, error) {
	if len(n.Errors) > 0 {
		return nil, n.Errors[0]
	}
	if n.Notification.Namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if n.Notification.Spec.Purpose == "" {
		return nil, errors.New("purpose is required")
	}
	if n.Notification.Spec.Channels == nil || len(n.Notification.Spec.Channels) == 0 {
		n.WithDefaultChannels(ctx, n.Notification.Namespace)
	}

	n.Notification.Name = makeName(n.Name, n.Notification)

	return n.Notification, nil
}

func (n *notificationBuilder) Send(ctx context.Context) (*notificationv1.Notification, error) {
	_, err := n.Build(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	k8sClient := cclient.ClientFromContextOrDie(ctx)
	notification := n.Notification.DeepCopy()

	mutator := func() error {
		if n.Owner != nil {
			err := controllerutil.SetControllerReference(n.Owner, notification, k8sClient.Scheme())
			if err != nil {
				return errors.Wrap(err, "failed to set controller reference")
			}
		}

		ensureLabels(notification)

		notification.Spec = n.Notification.Spec
		return nil
	}

	_, err = k8sClient.CreateOrUpdate(ctx, notification, mutator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send notification")
	}

	return notification, nil
}
