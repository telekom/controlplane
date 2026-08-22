// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/controller"
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
) ([]ctypes.ObjectRef, error) {
	rqSub, err := h.ensureBridgeSubscriber(ctx, listener, publisher, appId, callbackURL, apiBasePath, consumerId, providerId, "rq", "REQUEST")
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure bridge subscriber (rq)")
	}

	rpSub, err := h.ensureBridgeSubscriber(ctx, listener, publisher, appId, callbackURL, apiBasePath, consumerId, providerId, "rp", "RESPONSE")
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
	_ *spectrev1.Listener,
	publisher *pubsubv1.Publisher,
	appId string,
	callbackURL string,
	apiBasePath string,
	consumerId string,
	providerId string,
	kindSuffix string,
	kindValue string,
) (*pubsubv1.Subscriber, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	subscriberId := util.MakeBridgeSubscriberId(consumerId, appId, apiBasePath, kindSuffix)
	subscriber := &pubsubv1.Subscriber{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakeSubscriberName(subscriberId),
			Namespace: publisher.Namespace,
		},
	}

	mutator := func() error {
		subscriber.Spec = pubsubv1.SubscriberSpec{
			Publisher:    *ctypes.ObjectRefFromObject(publisher),
			SubscriberId: subscriberId,
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:     pubsubv1.DeliveryTypeCallback,
				Payload:  pubsubv1.PayloadTypeData,
				Callback: util.BuildBridgeCallbackURL(callbackURL, appId),
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

	_, err := c.CreateOrUpdate(ctx, subscriber, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update bridge Subscriber %q", subscriber.Name)
	}

	return subscriber, nil
}

// cleanupGenericPublisherIfOrphaned deletes the shared generic Publisher once the
// last Listener using it is gone.
//
// The Publisher is a single object per zone shared by every team, so the
// ref-count must span all namespaces — counting only the deleted Listener's own
// namespace would drop a Publisher that other teams are still using. The
// Listener being deleted is excluded by UID rather than by name, because names
// are only unique within a namespace.
func (h *ListenerHandler) cleanupGenericPublisherIfOrphaned(
	ctx context.Context,
	listener *spectrev1.Listener,
	zoneNamespace string,
) error {
	c := cclient.ClientFromContextOrDie(ctx)

	listenerList := &spectrev1.ListenerList{}
	if err := c.List(ctx, listenerList); err != nil {
		return errors.Wrap(err, "failed to list Listeners for publisher cleanup")
	}

	for i := range listenerList.Items {
		other := &listenerList.Items[i]
		if other.UID == listener.UID {
			continue
		}
		if controller.IsBeingDeleted(other) {
			continue
		}
		// Another Listener still needs the shared Publisher.
		return nil
	}

	publisherName := util.MakePublisherName(util.GenericEventType)
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
