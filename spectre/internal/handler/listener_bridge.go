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
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// ensureGenericPublisher creates or reuses the shared generic Publisher for all Spectre listeners.
// It is NOT owner-referenced since it is shared across Listener CRs, and it is
// never deleted: a draining Listener cannot know whether another Listener is
// about to subscribe to it, and a deleted Publisher blocks those Subscribers.
func (h *ListenerHandler) ensureGenericPublisher(
	ctx context.Context,
	eventStore *pubsubv1.EventStore,
) (*pubsubv1.Publisher, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	publisher := &pubsubv1.Publisher{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakePublisherName(util.GenericEventType),
			Namespace: eventStore.Namespace,
		},
	}

	mutator := func() error {
		if publisher.Labels == nil {
			publisher.Labels = make(map[string]string)
		}
		publisher.Labels[eventv1.EventTypeLabelKey] = labelutil.NormalizeLabelValue(util.GenericEventType)
		publisher.Spec = pubsubv1.PublisherSpec{
			EventStore:  *ctypes.ObjectRefFromObject(eventStore),
			EventType:   util.GenericEventType,
			PublisherId: util.PublisherID,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, publisher, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to ensure generic Publisher %q", publisher.Name)
	}

	return publisher, nil
}

// ensureBridgeSubscribers creates or updates the two bridge Subscribers (request + response)
// that connect the generic Publisher to the listener's callback endpoint.
func (h *ListenerHandler) ensureBridgeSubscribers(
	ctx context.Context,
	listener *spectrev1.Listener,
	publisher *pubsubv1.Publisher,
	appId string,
	callbackURL string,
	apiBasePath string,
	consumerId string,
	providerId string,
	fingerprint string,
) ([]ctypes.ObjectRef, error) {
	rqSub, err := h.ensureBridgeSubscriber(ctx, listener, publisher, appId, callbackURL, apiBasePath, consumerId, providerId, "rq", "REQUEST", fingerprint, listener.Spec.ApiListener.RequestFilter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure bridge subscriber (rq)")
	}

	rpSub, err := h.ensureBridgeSubscriber(ctx, listener, publisher, appId, callbackURL, apiBasePath, consumerId, providerId, "rp", "RESPONSE", fingerprint, listener.Spec.ApiListener.ResponseFilter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure bridge subscriber (rp)")
	}

	return []ctypes.ObjectRef{
		*ctypes.ObjectRefFromObject(rqSub),
		*ctypes.ObjectRefFromObject(rpSub),
	}, nil
}

func (h *ListenerHandler) ensureBridgeSubscriber(
	ctx context.Context,
	listener *spectrev1.Listener,
	publisher *pubsubv1.Publisher,
	appId string,
	callbackURL string,
	apiBasePath string,
	consumerId string,
	providerId string,
	kindSuffix string,
	kindValue string,
	fingerprint string,
	filter *spectrev1.ListenerFilter,
) (*pubsubv1.Subscriber, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	subscriberId := util.MakeBridgeSubscriberId(consumerId, appId, apiBasePath, kindSuffix)
	subscriber := &pubsubv1.Subscriber{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakeSubscriberName(subscriberId),
			Namespace: publisher.Namespace,
		},
	}

	gatewayCallback, err := util.BuildBridgeCallbackURL(callbackURL, appId)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build bridge callback URL for subscriber %q", subscriberId)
	}

	mutator := func() error {
		if subscriber.Labels == nil {
			subscriber.Labels = make(map[string]string)
		}
		subscriber.Labels[cconfig.OwnerUidLabelKey] = string(listener.UID)
		subscriber.Labels[AuthorizationFingerprintLabelKey] = fingerprint
		subscriber.Labels[eventv1.EventTypeLabelKey] = labelutil.NormalizeLabelValue(util.GenericEventType)
		subscriber.Spec = pubsubv1.SubscriberSpec{
			Publisher:    *ctypes.ObjectRefFromObject(publisher),
			SubscriberId: subscriberId,
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:     pubsubv1.DeliveryTypeCallback,
				Payload:  pubsubv1.PayloadTypeData,
				Callback: gatewayCallback,
			},
			Trigger: buildBridgeTrigger(filter, apiBasePath, consumerId, providerId, kindValue),
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, subscriber, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update bridge Subscriber %q", subscriber.Name)
	}

	return subscriber, nil
}
