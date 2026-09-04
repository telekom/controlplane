// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// ensurePublisher creates or updates the pubsub Publisher for this SpectreApplication.
func (h *SpectreApplicationHandler) ensurePublisher(ctx context.Context, obj *spectrev1.SpectreApplication, eventStore *pubsubv1.EventStore, appId string) (*pubsubv1.Publisher, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventType := util.BuildListenerEventType(appId)
	publisher := &pubsubv1.Publisher{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakePublisherName(eventType),
			Namespace: eventStore.Namespace,
		},
	}

	mutator := func() error {
		if publisher.Labels == nil {
			publisher.Labels = make(map[string]string)
		}
		publisher.Labels[cconfig.OwnerUidLabelKey] = string(obj.UID)
		publisher.Spec = pubsubv1.PublisherSpec{
			EventStore:  *ctypes.ObjectRefFromObject(eventStore),
			EventType:   eventType,
			PublisherId: util.PublisherID,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, publisher, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Publisher %q", publisher.Name)
	}

	return publisher, nil
}

// ensureSubscriber creates or updates the pubsub Subscriber for this SpectreApplication.
func (h *SpectreApplicationHandler) ensureSubscriber(ctx context.Context, obj *spectrev1.SpectreApplication, publisher *pubsubv1.Publisher, appId, gatewayCallbackURL string) (*pubsubv1.Subscriber, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	delivery, err := mapDelivery(obj, gatewayCallbackURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build delivery spec")
	}

	subscriber := &pubsubv1.Subscriber{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakeSubscriberName(appId),
			Namespace: publisher.Namespace,
		},
	}

	mutator := func() error {
		if subscriber.Labels == nil {
			subscriber.Labels = make(map[string]string)
		}
		subscriber.Labels[cconfig.OwnerUidLabelKey] = string(obj.UID)
		subscriber.Spec = pubsubv1.SubscriberSpec{
			Publisher:    *ctypes.ObjectRefFromObject(publisher),
			SubscriberId: appId,
			Delivery:     delivery,
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, subscriber, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Subscriber %q", subscriber.Name)
	}

	return subscriber, nil
}

// mapDelivery converts a SpectreApplication's delivery config into a pubsub SubscriptionDelivery.
// For callback delivery the raw customer callback is routed through the Gateway
// so that Horizon never contacts the external endpoint directly.
func mapDelivery(obj *spectrev1.SpectreApplication, gatewayCallbackURL string) (pubsubv1.SubscriptionDelivery, error) {
	delivery := pubsubv1.SubscriptionDelivery{
		Payload: pubsubv1.PayloadTypeData,
	}

	switch obj.Spec.DeliveryType {
	case "server_sent_event":
		delivery.Type = pubsubv1.DeliveryTypeServerSentEvent
	case "callback":
		delivery.Type = pubsubv1.DeliveryTypeCallback
		cb, err := util.BuildGatewayCallbackURL(gatewayCallbackURL, obj.Spec.Callback)
		if err != nil {
			return delivery, errors.Wrap(err, "failed to build gateway callback URL")
		}
		delivery.Callback = cb
	default:
		delivery.Type = pubsubv1.DeliveryTypeServerSentEvent
	}

	return delivery, nil
}
