// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// ensureGenericPublisher creates or reuses the shared generic Publisher for all Spectre listeners.
// It is NOT owner-referenced since it is shared across Listener CRs.
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
	rqSub, err := h.ensureBridgeSubscriber(ctx, listener, publisher, appId, callbackURL, apiBasePath, consumerId, providerId, "rq", "REQUEST", fingerprint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure bridge subscriber (rq)")
	}

	rpSub, err := h.ensureBridgeSubscriber(ctx, listener, publisher, appId, callbackURL, apiBasePath, consumerId, providerId, "rp", "RESPONSE", fingerprint)
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
		subscriber.Spec = pubsubv1.SubscriberSpec{
			Publisher:    *ctypes.ObjectRefFromObject(publisher),
			SubscriberId: subscriberId,
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:     pubsubv1.DeliveryTypeCallback,
				Payload:  pubsubv1.PayloadTypeData,
				Callback: gatewayCallback,
			},
			Trigger: &pubsubv1.Trigger{
				SelectionFilter: &pubsubv1.SelectionFilter{
					Attributes: map[string]string{
						"issue":    apiBasePath,
						"consumer": consumerId,
						"provider": providerId,
						"kind":     kindValue,
					},
				},
			},
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, subscriber, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update bridge Subscriber %q", subscriber.Name)
	}

	return subscriber, nil
}

// cleanupGenericPublisherIfOrphaned deletes the shared generic Publisher once no
// Subscriber in the zone namespace references it.
//
// Instead of counting global Listeners (which misses cross-zone and leaves stale
// Publishers), this lists Subscribers in the actual zone namespace and checks
// whether any still reference the generic Publisher. Terminating Subscribers
// (with finalizers) are counted too — their finalizers still require the
// Publisher.
func (h *ListenerHandler) cleanupGenericPublisherIfOrphaned(
	ctx context.Context,
	zoneNamespace string,
) error {
	c := cclient.ClientFromContextOrDie(ctx)

	publisherName := util.MakePublisherName(util.GenericEventType)
	publisherRef := ctypes.ObjectRef{
		Name:      publisherName,
		Namespace: zoneNamespace,
	}

	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, client.InNamespace(zoneNamespace)); err != nil {
		return errors.Wrap(err, "failed to list Subscribers for publisher cleanup")
	}

	for i := range subList.Items {
		sub := &subList.Items[i]
		if sub.Spec.Publisher.Name == publisherRef.Name && sub.Spec.Publisher.Namespace == publisherRef.Namespace {
			// A Subscriber still references this Publisher (including terminating ones).
			return nil
		}
	}

	publisher := &pubsubv1.Publisher{
		ObjectMeta: metav1.ObjectMeta{
			Name:      publisherName,
			Namespace: zoneNamespace,
		},
	}

	if err := c.Delete(ctx, publisher); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete orphaned generic Publisher %q", publisherName)
	}

	return nil
}
