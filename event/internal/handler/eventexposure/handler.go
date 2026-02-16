// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	"context"

	"github.com/pkg/errors"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*eventv1.EventExposure] = &EventExposureHandler{}

type EventExposureHandler struct{}

func (h *EventExposureHandler) CreateOrUpdate(ctx context.Context, obj *eventv1.EventExposure) error {
	logger := log.FromContext(ctx)

	found, _, err := util.FindActiveEventType(ctx, obj.Spec.EventType)
	if err != nil {
		return err
	}
	if !found {
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("EventTypeNotFound",
			"No active EventType found for type "+obj.Spec.EventType))
		obj.SetCondition(condition.NewBlockedCondition(
			"EventType " + obj.Spec.EventType + " does not exist or is not active. " +
				"EventExposure will be automatically processed when the EventType is registered"))
		return nil
	}

	existingExposures, err := util.FindEventExposures(ctx, obj.Spec.EventType)
	if err != nil {
		return errors.Wrapf(err, "failed to list EventExposures for event type %q", obj.Spec.EventType)
	}
	existingFound, existingExposure, err := util.FindActiveEventExposure(existingExposures)
	if err != nil {
		return errors.Wrapf(err, "failed to find active EventExposure for event type %q", obj.Spec.EventType)
	}

	if existingFound && existingExposure.UID != obj.UID {
		// Another exposure already owns this event type
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("EventExposureAlreadyExists",
			"Event type "+obj.Spec.EventType+" is already exposed by "+existingExposure.Name))
		obj.SetCondition(condition.NewBlockedCondition(
			"Event already exposed by " + existingExposure.Name + ". " +
				"Only one active EventExposure per event type is allowed"))
		return nil
	}

	// This exposure is the active one (either no existing, or we are the existing one)
	obj.Status.Active = true

	// TODO: Validate category — check if the provider's team category allows exposure of this event category
	// TODO: Validate visibility — check if exposure visibility is compatible with zone visibility

	zone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return err
	}

	eventConfig, err := util.GetEventConfigForZone(ctx, obj.Spec.Zone.Name)
	if err != nil {
		return err
	}
	obj.Status.CallbackURL = eventConfig.Status.CallbackURL

	eventStore, err := util.GetEventStoreForZone(ctx, obj.Spec.Zone.Name)
	if err != nil {
		return err
	}

	application, err := util.GetApplication(ctx, obj.Spec.Provider.ObjectRef)
	if err != nil {
		return errors.Wrap(err, "failed to get application")
	}

	publisher, err := h.createPublisher(ctx, obj, eventStore, application)
	if err != nil {
		return errors.Wrap(err, "failed to create Publisher")
	}
	obj.Status.Publisher = types.ObjectRefFromObject(publisher)
	logger.V(1).Info("Publisher created/updated", "publisher", publisher.Name)

	// --- SSE Route management ---

	crossZones, err := util.FindCrossZoneSSESubscriptionZones(ctx, obj.Spec.EventType, obj.Spec.Zone.Name)
	if err != nil {
		return errors.Wrap(err, "failed to find cross-zone SSE subscriptions")
	}

	obj.Status.ProxyRoutes = nil
	obj.Status.SseURLs = make(map[string]string)
	for _, subscriberZoneRef := range crossZones {
		subscriberZone, err := util.GetZone(ctx, subscriberZoneRef.K8s())
		if err != nil {
			return errors.Wrapf(err, "failed to get subscriber zone %q", subscriberZoneRef.Name)
		}

		proxyRoute, err := util.CreateSSEProxyRoute(ctx, obj.Spec.EventType, eventConfig, subscriberZone, zone)
		if err != nil {
			return errors.Wrapf(err, "failed to create SSE proxy Route for zone %q", subscriberZoneRef.Name)
		}
		obj.Status.ProxyRoutes = append(obj.Status.ProxyRoutes, *types.ObjectRefFromObject(proxyRoute))
		obj.Status.SseURLs[subscriberZoneRef.Name] = proxyRoute.Spec.Downstreams[0].Url()
		logger.V(1).Info("SSE proxy Route created/updated", "zone", subscriberZoneRef.Name, "route", proxyRoute.Name)
	}

	isProxyTarget := len(obj.Status.ProxyRoutes) > 0
	route, err := util.CreateSSERoute(ctx, obj.Spec.EventType, zone, eventConfig, isProxyTarget)
	if err != nil {
		return errors.Wrap(err, "failed to create SSE Route")
	}
	obj.Status.Route = types.ObjectRefFromObject(route)
	obj.Status.SseURLs[zone.Name] = route.Spec.Downstreams[0].Url()

	deleted, err := util.CleanupOldSSERoutes(ctx, obj.Spec.EventType)
	if err != nil {
		return errors.Wrap(err, "failed to cleanup old SSE Routes")
	}
	if deleted > 0 {
		logger.V(1).Info("Cleaned up stale SSE Routes", "deleted", deleted)
	}

	// 9. Set final conditions
	c := cclient.ClientFromContextOrDie(ctx)
	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady",
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("EventExposureProvisioned",
		"EventExposure has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"EventExposure has been provisioned"))

	return nil
}

func (h *EventExposureHandler) Delete(ctx context.Context, obj *eventv1.EventExposure) error {
	logger := log.FromContext(ctx)

	// Publisher is cleaned up automatically via ownerRef (SetControllerReference).

	// Check if another EventExposure exists for the same event type.
	// If so, skip Route deletion — the other exposure will take over.
	otherExists, err := util.AnyOtherEventExposureExists(ctx, obj.Spec.EventType, obj.UID)
	if err != nil {
		return errors.Wrap(err, "failed to check for other EventExposures")
	}

	if otherExists {
		// Another exposure exists — it will take over the Route via
		// MapEventExposureToEventExposure watch + re-reconciliation.
		logger.Info("Skipping Route deletion — another EventExposure exists for this event type",
			"eventType", obj.Spec.EventType)
		return nil
	}

	if obj.Status.Publisher != nil {
		c := cclient.ClientFromContextOrDie(ctx)
		publisher := &pubsubv1.Publisher{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obj.Status.Publisher.Name,
				Namespace: obj.Status.Publisher.Namespace,
			},
		}
		err = c.Delete(ctx, publisher)
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete Publisher %q", obj.Status.Publisher.String())
		}
		logger.Info("Deleted Publisher", "publisher", obj.Status.Publisher.String())
	}

	// Last exposure for this event type — clean up the Route and proxy Routes.
	if obj.Status.Route != nil {
		if err := util.DeleteRouteIfExists(ctx, obj.Status.Route); err != nil {
			return errors.Wrap(err, "failed to delete SSE Route")
		}
		logger.Info("Deleted SSE Route", "route", obj.Status.Route.String())
	}

	for i := range obj.Status.ProxyRoutes {
		ref := &obj.Status.ProxyRoutes[i]
		if err := util.DeleteRouteIfExists(ctx, ref); err != nil {
			return errors.Wrapf(err, "failed to delete SSE proxy Route %q", ref.String())
		}
		logger.Info("Deleted SSE proxy Route", "route", ref.String())
	}

	return nil
}

// createPublisher creates a pubsub.Publisher child resource for this EventExposure.
func (h *EventExposureHandler) createPublisher(ctx context.Context, obj *eventv1.EventExposure, eventStore *pubsubv1.EventStore, application *applicationv1.Application) (*pubsubv1.Publisher, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	publisher := &pubsubv1.Publisher{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(obj.Spec.EventType),
			Namespace: eventStore.Namespace, // zone namespace
		},
	}
	publisherId := application.Status.ClientId

	mutator := func() error {
		publisher.Labels = map[string]string{
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(application.Name),
			eventv1.EventTypeLabelKey:           labelutil.NormalizeLabelValue(obj.Spec.EventType),
			config.BuildLabelKey("zone"):        obj.Spec.Zone.Name,
		}

		publisher.Spec = pubsubv1.PublisherSpec{
			EventStore:             *types.ObjectRefFromObject(eventStore),
			EventType:              obj.Spec.EventType,
			PublisherId:            publisherId,
			AdditionalPublisherIds: obj.Spec.AdditionalPublisherIds,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, publisher, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Publisher %q", obj.Name)
	}

	return publisher, nil
}
