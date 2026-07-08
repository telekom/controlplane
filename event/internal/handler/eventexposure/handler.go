// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

var _ handler.Handler[*eventv1.EventExposure] = &EventExposureHandler{}

type EventExposureHandler struct{}

func (h *EventExposureHandler) CreateOrUpdate(ctx context.Context, obj *eventv1.EventExposure) error {
	logger := log.FromContext(ctx)

	found, eventType, err := util.FindActiveEventType(ctx, obj.Spec.EventType)
	if err != nil {
		return err
	}
	if !found {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet,
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
		msg := fmt.Sprintf("Event-Type %q is already exposed by team %q.", obj.Spec.EventType, existingExposure.Spec.Provider.Namespace)
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, msg))
		obj.SetCondition(condition.NewBlockedCondition(msg + " EventExposure will be automatically processed when the existing EventExposure is deleted"))
		return nil
	}

	// This exposure is the active one (either no existing, or we are the existing one)
	obj.Status.Active = true

	// TODO: Validate category — check if the provider's team category allows exposure of this event category

	zone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return err
	}

	eventConfig, err := util.GetEventConfigForZone(ctx, obj.Spec.Zone.Name)
	if err != nil {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, "Event Feature has not been fully provisioned for this zone yet"))
		return err
	}
	obj.Status.CallbackURL = eventConfig.Status.CallbackURL
	logger.V(1).Info("Found EventConfig for zone", "zone", obj.Spec.Zone.Name, "eventConfig", eventConfig.Name)

	eventStore, err := util.GetEventStoreForZone(ctx, obj.Spec.Zone.Name)
	if err != nil {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, "Event Feature has not been fully provisioned for this zone yet"))
		return err
	}
	logger.V(1).Info("Found EventStore for zone", "zone", obj.Spec.Zone.Name, "eventStore", eventStore.Name)

	application, err := util.GetApplication(ctx, obj.Spec.Provider.ObjectRef)
	if err != nil {
		return errors.Wrap(err, "failed to get application")
	}
	logger.V(1).Info("Found provider application", "application", application.Name)

	publisher, err := h.createPublisher(ctx, obj, eventType, eventStore, application)
	if err != nil {
		return errors.Wrap(err, "failed to create Publisher")
	}
	obj.Status.Publisher = types.ObjectRefFromObject(publisher)
	logger.V(1).Info("Publisher created/updated", "publisher", publisher.Name)

	// --- SSE Route management ---
	if err := h.reconcileSSERoutes(ctx, obj, zone, eventConfig); err != nil {
		return err
	}

	// 9. Set final conditions
	c := cclient.ClientFromContextOrDie(ctx)
	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady, "Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned,
		"EventExposure has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"EventExposure has been provisioned"))

	return nil
}

// reconcileSSERoutes manages SSE Route creation for cross-zone proxy routes and the primary route.
func (h *EventExposureHandler) reconcileSSERoutes(ctx context.Context, obj *eventv1.EventExposure, zone *adminv1.Zone, eventConfig *eventv1.EventConfig) error {
	logger := log.FromContext(ctx)
	realmName := zone.Status.RealmName

	obj.Status.Route = nil
	obj.Status.ProxyRoutes = nil
	obj.Status.SseURLs = make(map[string]string)

	// Resolve the backend (local) zone that actually runs Horizon. For a local
	// exposure zone this is the zone itself; for a proxy exposure zone it is the
	// proxy's target zone, where the SSE backend and primary Route live. The proxy
	// zone then gets an own-zone proxy Route forwarding to the backend zone.
	backendZone, backendConfig, err := h.resolveSSEBackendZone(ctx, zone, eventConfig)
	if err != nil {
		return err
	}

	crossZones, err := util.FindCrossZoneSSESubscriptionZones(ctx, obj.Spec.EventType, obj.Spec.Zone.Name)
	if err != nil {
		return errors.Wrap(err, "failed to find cross-zone SSE subscriptions")
	}

	// Subscriber proxy Routes forward to the backend zone (not the exposure zone,
	// which may itself be a proxy). The backend zone is served directly by the
	// primary Route below, so it is skipped inside createProxySSERoutes.
	subscriberZones, err := h.createProxySSERoutes(ctx, obj, backendZone, crossZones, realmName)
	if err != nil {
		return err
	}

	// A proxy exposure zone needs its own SSE Route forwarding to the backend zone.
	// This is the same shape as a cross-zone subscriber proxy Route (proxy Route in
	// the zone's namespace, upstream = backend zone's gateway SSE path).
	if eventConfig.IsProxy() {
		// Subscribers in this zone connect to the own-zone proxy Route directly via the
		// local alias path with their IDP token, so trust the zone's IDP issuer. The mesh
		// hop to the backend zone is authenticated separately (LMS issuer on the primary).
		var proxyTrustedIssuers []string
		if zone.Status.Links.Issuer != "" {
			proxyTrustedIssuers = []string{zone.Status.Links.Issuer}
		}
		ownProxyRoute, routeErr := util.CreateSSEProxyRoute(ctx, obj.Spec.EventType, zone, backendZone,
			util.WithTrustedIssuers(proxyTrustedIssuers),
			util.WithRealmName(realmName),
		)
		if routeErr != nil {
			return errors.Wrap(routeErr, "failed to create own-zone proxy SSE Route")
		}
		obj.Status.ProxyRoutes = append(obj.Status.ProxyRoutes, *types.ObjectRefFromObject(ownProxyRoute))
		obj.Status.SseURLs[zone.Name] = util.RouteDownstreamURL(ownProxyRoute)
		// The exposure proxy zone fronts the primary Route just like a subscriber
		// proxy zone, so its LMS issuer must be trusted by the primary.
		subscriberZones = append(subscriberZones, zone)
		logger.V(1).Info("Own-zone proxy SSE Route created/updated", "zone", zone.Name, "route", ownProxyRoute.Name)
	}

	// Primary SSE route in the backend zone: trusted issuers = [backend IDP issuer] + [LMS issuers from proxy zones].
	isProxyTarget := len(obj.Status.ProxyRoutes) > 0
	primaryTrustedIssuers := collectPrimaryTrustedIssuers(backendZone, subscriberZones, isProxyTarget)

	route, err := util.CreateSSERoute(ctx, obj.Spec.EventType, backendZone, backendConfig, isProxyTarget,
		util.WithTrustedIssuers(primaryTrustedIssuers),
		util.WithRealmName(backendZone.Status.RealmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create SSE Route")
	}
	obj.Status.Route = types.ObjectRefFromObject(route)
	obj.Status.SseURLs[backendZone.Name] = util.RouteDownstreamURL(route)

	deleted, err := util.CleanupOldSSERoutes(ctx, obj.Spec.EventType)
	if err != nil {
		return errors.Wrap(err, "failed to cleanup old SSE Routes")
	}
	if deleted > 0 {
		logger.V(1).Info("Cleaned up stale SSE Routes", "deleted", deleted)
	}

	return nil
}

// resolveSSEBackendZone returns the zone (and its EventConfig) that runs the local
// Horizon SSE backend for this exposure. For a local exposure zone that is the zone
// itself. For a proxy exposure zone it resolves the proxy's target zone, which must
// be a ready local (non-proxy) zone.
func (h *EventExposureHandler) resolveSSEBackendZone(ctx context.Context, zone *adminv1.Zone, eventConfig *eventv1.EventConfig) (*adminv1.Zone, *eventv1.EventConfig, error) {
	if !eventConfig.IsProxy() {
		return zone, eventConfig, nil
	}

	targetZoneName := eventConfig.Spec.Proxy.TargetZone.Name
	targetCfg, err := util.GetEventConfigForZone(ctx, targetZoneName)
	if err != nil {
		return nil, nil, err // BlockedError propagates so the proxy requeues until the target is ready
	}
	if targetCfg.IsProxy() || targetCfg.Spec.Local == nil {
		return nil, nil, ctrlerrors.BlockedErrorf("target zone %q of proxy zone %q must be a local (non-proxy) zone", targetZoneName, zone.Name)
	}

	targetZone, err := util.GetZone(ctx, targetCfg.Spec.Zone.K8s())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get target zone %q", targetZoneName)
	}
	return targetZone, targetCfg, nil
}

// createProxySSERoutes creates proxy SSE routes for cross-zone subscribers and returns the subscriber zones.
// backendZone is the zone running the SSE backend that all proxy routes forward to.
func (h *EventExposureHandler) createProxySSERoutes(ctx context.Context, obj *eventv1.EventExposure, backendZone *adminv1.Zone, crossZones []types.ObjectRef, realmName string) ([]*adminv1.Zone, error) {
	logger := log.FromContext(ctx)

	var subscriberZones []*adminv1.Zone
	for _, subscriberZoneRef := range crossZones {
		// The backend zone is served directly by the primary Route; never proxy to itself.
		if subscriberZoneRef.Name == backendZone.Name {
			continue
		}

		subscriberZone, zoneErr := util.GetZone(ctx, subscriberZoneRef.K8s())
		if zoneErr != nil {
			return nil, errors.Wrapf(zoneErr, "failed to get subscriber zone %q", subscriberZoneRef.Name)
		}
		subscriberZones = append(subscriberZones, subscriberZone)

		// Subscribers connect to this proxy Route directly via the local alias path with
		// their own zone's IDP token, so trust the subscriber zone's IDP issuer. The mesh
		// hop to the backend zone is authenticated with the LMS issuer, trusted on the primary.
		var proxyTrustedIssuers []string
		if subscriberZone.Status.Links.Issuer != "" {
			proxyTrustedIssuers = []string{subscriberZone.Status.Links.Issuer}
		}

		proxyRoute, routeErr := util.CreateSSEProxyRoute(ctx, obj.Spec.EventType, subscriberZone, backendZone,
			util.WithTrustedIssuers(proxyTrustedIssuers),
			util.WithRealmName(realmName),
		)
		if routeErr != nil {
			return nil, errors.Wrapf(routeErr, "failed to create SSE proxy Route for zone %q", subscriberZoneRef.Name)
		}
		obj.Status.ProxyRoutes = append(obj.Status.ProxyRoutes, *types.ObjectRefFromObject(proxyRoute))
		obj.Status.SseURLs[subscriberZoneRef.Name] = util.RouteDownstreamURL(proxyRoute)
		logger.V(1).Info("SSE proxy Route created/updated", "zone", subscriberZoneRef.Name, "route", proxyRoute.Name)
	}

	return subscriberZones, nil
}

// collectPrimaryTrustedIssuers builds the trusted issuer list for the primary SSE route.
func collectPrimaryTrustedIssuers(zone *adminv1.Zone, subscriberZones []*adminv1.Zone, isProxyTarget bool) []string {
	var issuers []string
	if zone.Status.Links.Issuer != "" {
		issuers = append(issuers, zone.Status.Links.Issuer)
	}
	if isProxyTarget {
		for _, subZone := range subscriberZones {
			if subZone.Status.Links.LmsIssuer != "" {
				issuers = append(issuers, subZone.Status.Links.LmsIssuer)
			}
		}
	}
	return issuers
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
func (h *EventExposureHandler) createPublisher(ctx context.Context, obj *eventv1.EventExposure, eventType *eventv1.EventType, eventStore *pubsubv1.EventStore, application *applicationv1.Application) (*pubsubv1.Publisher, error) {
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
			JsonSchema:             eventType.Spec.Specification,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, publisher, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Publisher %q", obj.Name)
	}

	return publisher, nil
}
