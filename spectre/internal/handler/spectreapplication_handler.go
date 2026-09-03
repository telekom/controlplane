// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

type SpectreApplicationHandler struct{}

func (h *SpectreApplicationHandler) CreateOrUpdate(ctx context.Context, obj *spectrev1.SpectreApplication) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Step 1: Resolve the referenced Application to get the appId.
	application, err := h.resolveApplication(ctx, obj)
	if err != nil {
		return err
	}
	appId := application.Status.ClientId
	obj.Status.Id = appId
	logger.Info("Resolved Application", "appId", appId)

	// Step 2: Resolve Zone -> get EventConfig + find EventStore.
	zone, err := h.resolveZone(ctx, application)
	if err != nil {
		return errors.Wrap(err, "failed to resolve zone")
	}

	eventConfig, err := util.GetEventConfig(ctx, zone)
	if err != nil {
		return errors.Wrap(err, "failed to get EventConfig")
	}

	eventStore, err := util.ResolveEventStore(ctx, eventConfig)
	if err != nil {
		return errors.Wrap(err, "failed to resolve EventStore")
	}

	// Step 2.5: Validate CallbackURL when callback delivery is configured.
	gatewayCallbackURL := eventConfig.Status.CallbackURL
	if obj.Spec.DeliveryType == "callback" && gatewayCallbackURL == "" {
		return ctrlerrors.BlockedErrorf("EventConfig %q has no CallbackURL in status — required for callback delivery", eventConfig.Name)
	}

	// Step 3: Ensure Publisher.
	publisher, err := h.ensurePublisher(ctx, obj, eventStore, appId)
	if err != nil {
		return errors.Wrap(err, "failed to ensure Publisher")
	}
	obj.Status.Publisher = ctypes.ObjectRefFromObject(publisher)
	logger.Info("Ensured Publisher", "publisher", publisher.Name)

	// Step 4: Ensure Subscriber.
	subscriber, err := h.ensureSubscriber(ctx, obj, publisher, appId, gatewayCallbackURL)
	if err != nil {
		return errors.Wrap(err, "failed to ensure Subscriber")
	}
	obj.Status.Subscriber = ctypes.ObjectRefFromObject(subscriber)
	logger.Info("Ensured Subscriber", "subscriber", subscriber.Name)

	// Step 5: If SSE delivery, ensure SSE Route.
	if obj.Spec.DeliveryType == "server_sent_event" {
		route, err := h.ensureSSERoute(ctx, obj, zone, eventConfig, appId)
		if err != nil {
			return errors.Wrap(err, "failed to ensure SSE Route")
		}
		obj.Status.ListenerRoute = ctypes.ObjectRefFromObject(route)
		logger.Info("Ensured SSE Route", "route", route.Name)
	}

	// Step 5.5: Cleanup obsolete children that were not touched in this reconcile.
	// Order: Routes first (stop stale SSE access), then Subscribers, then Publishers.
	// Route cleanup runs for all delivery types — this handles SSE→callback transition.
	if _, err := c.Cleanup(ctx, &gatewayv1.RouteList{}, cclient.OwnedByLabel(obj)); err != nil {
		return errors.Wrap(err, "failed to cleanup obsolete Routes")
	}
	if _, err := c.Cleanup(ctx, &pubsubv1.SubscriberList{}, cclient.OwnedByLabel(obj)); err != nil {
		return errors.Wrap(err, "failed to cleanup obsolete Subscribers")
	}
	if _, err := c.Cleanup(ctx, &pubsubv1.PublisherList{}, cclient.OwnedByLabel(obj)); err != nil {
		return errors.Wrap(err, "failed to cleanup obsolete Publishers")
	}

	// Step 6: Set Ready condition.
	// AllReady() only turns false once a child reports Ready=False. A child that
	// was just created has no conditions at all, so check AnyChanged() first —
	// otherwise the first reconcile reports Ready before anything is confirmed.
	if c.AnyChanged() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"At least one sub-resource has been created or updated"))
		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady,
			"At least one sub-resource has been created or updated"))
		return nil
	}

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned,
		"SpectreApplication has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("SpectreApplication has been provisioned"))

	return nil
}

func (h *SpectreApplicationHandler) Delete(ctx context.Context, obj *spectrev1.SpectreApplication) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Phase 1: Delete all Subscribers (status ref + owner-labelled).
	if ref := obj.Status.Subscriber; ref != nil {
		sub := &pubsubv1.Subscriber{}
		if err := deleteIfExists(ctx, c, ref, sub); err != nil {
			return errors.Wrapf(err, "failed to delete Subscriber %q", ref.String())
		}
		logger.Info("Deleted Subscriber", "subscriber", ref.String())
	}
	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(obj)...); err != nil {
		return errors.Wrap(err, "failed to list owned Subscribers")
	}
	for i := range subList.Items {
		sub := &subList.Items[i]
		if err := c.Delete(ctx, sub); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete Subscriber %q", sub.Name)
		}
		logger.Info("Deleted owned Subscriber", "subscriber", sub.Name, "namespace", sub.Namespace)
	}

	// Fresh-list: retry while any Subscribers remain (finalizers still running).
	remainingSubs := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, remainingSubs, cclient.OwnedByLabel(obj)...); err != nil {
		return errors.Wrap(err, "failed to re-list owned Subscribers")
	}
	if len(remainingSubs.Items) > 0 {
		return ctrlerrors.RetryableWithDelayErrorf(
			2*time.Second,
			"waiting for Subscriber finalization before deleting Publishers",
		)
	}
	obj.Status.Subscriber = nil

	// Phase 2: Delete all Publishers (status ref + owner-labelled).
	if ref := obj.Status.Publisher; ref != nil {
		pub := &pubsubv1.Publisher{}
		if err := deleteIfExists(ctx, c, ref, pub); err != nil {
			return errors.Wrapf(err, "failed to delete Publisher %q", ref.String())
		}
		logger.Info("Deleted Publisher", "publisher", ref.String())
	}
	pubList := &pubsubv1.PublisherList{}
	if err := c.List(ctx, pubList, cclient.OwnedByLabel(obj)...); err != nil {
		return errors.Wrap(err, "failed to list owned Publishers")
	}
	for i := range pubList.Items {
		pub := &pubList.Items[i]
		if err := c.Delete(ctx, pub); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete Publisher %q", pub.Name)
		}
		logger.Info("Deleted owned Publisher", "publisher", pub.Name, "namespace", pub.Namespace)
	}
	obj.Status.Publisher = nil

	// Phase 3: Delete all Routes (status refs + owner-labelled).
	for _, ref := range []*ctypes.ObjectRef{obj.Status.ListenerRoute, obj.Status.ProxyRoute} {
		if ref == nil {
			continue
		}
		route := &gatewayv1.Route{}
		if err := deleteIfExists(ctx, c, ref, route); err != nil {
			return errors.Wrapf(err, "failed to delete Route %q", ref.String())
		}
		logger.Info("Deleted Route", "route", ref.String())
	}
	routeList := &gatewayv1.RouteList{}
	if err := c.List(ctx, routeList, cclient.OwnedByLabel(obj)...); err != nil {
		return errors.Wrap(err, "failed to list owned Routes")
	}
	for i := range routeList.Items {
		route := &routeList.Items[i]
		if err := c.Delete(ctx, route); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete Route %q", route.Name)
		}
		logger.Info("Deleted owned Route", "route", route.Name, "namespace", route.Namespace)
	}
	obj.Status.ListenerRoute = nil
	obj.Status.ProxyRoute = nil

	return nil
}

// deleteIfExists deletes the referenced object, tolerating an already-deleted one.
func deleteIfExists(ctx context.Context, c cclient.JanitorClient, ref *ctypes.ObjectRef, into client.Object) error {
	if err := c.Get(ctx, ref.K8s(), into); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := c.Delete(ctx, into); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// resolveApplication fetches the referenced Application and ensures it is ready.
func (h *SpectreApplicationHandler) resolveApplication(ctx context.Context, obj *spectrev1.SpectreApplication) (*applicationv1.Application, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	app := &applicationv1.Application{}
	ref := obj.Spec.Application.ObjectRef
	err := c.Get(ctx, ref.K8s(), app)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q not found: %v", ref.String(), err)
	}

	if err := condition.EnsureReady(app); err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q is not ready", ref.String())
	}

	return app, nil
}

// resolveZone fetches the Zone referenced by the Application and ensures it is ready.
func (h *SpectreApplicationHandler) resolveZone(ctx context.Context, app *applicationv1.Application) (*adminv1.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zone := &adminv1.Zone{}
	err := c.Get(ctx, app.Spec.Zone.K8s(), zone)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q not found: %v", app.Spec.Zone.String(), err)
	}

	if err := condition.EnsureReady(zone); err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q is not ready", app.Spec.Zone.String())
	}

	return zone, nil
}
