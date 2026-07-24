// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
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
	appId := application.Name
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

	eventStore, err := h.findEventStore(ctx, zone.Status.Namespace)
	if err != nil {
		return err
	}

	// Step 3: Ensure Publisher.
	publisher, err := h.ensurePublisher(ctx, obj, eventStore, appId)
	if err != nil {
		return errors.Wrap(err, "failed to ensure Publisher")
	}
	obj.Status.Publisher = ctypes.ObjectRefFromObject(publisher)
	logger.Info("Ensured Publisher", "publisher", publisher.Name)

	// Step 4: Ensure Subscriber.
	subscriber, err := h.ensureSubscriber(ctx, obj, publisher, appId)
	if err != nil {
		return errors.Wrap(err, "failed to ensure Subscriber")
	}
	obj.Status.Subscriber = ctypes.ObjectRefFromObject(subscriber)
	logger.Info("Ensured Subscriber", "subscriber", subscriber.Name)

	// Step 5: If SSE delivery, ensure SSE Route.
	if obj.Spec.DeliveryType == "server_sent_event" {
		route, err := h.ensureSSERoute(ctx, zone, eventConfig, appId)
		if err != nil {
			return errors.Wrap(err, "failed to ensure SSE Route")
		}
		obj.Status.ListenerRoute = ctypes.ObjectRefFromObject(route)
		logger.Info("Ensured SSE Route", "route", route.Name)
	}

	// Step 6: Set Ready condition.
	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned,
		"SpectreApplication has been provisioned"))

	return nil
}

func (h *SpectreApplicationHandler) Delete(ctx context.Context, obj *spectrev1.SpectreApplication) error {
	// Child resources are cleaned up via owner references.
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

// findEventStore lists EventStore CRs in the zone namespace and returns the single expected one.
func (h *SpectreApplicationHandler) findEventStore(ctx context.Context, zoneNamespace string) (*pubsubv1.EventStore, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventStoreList := &pubsubv1.EventStoreList{}
	err := c.List(ctx, eventStoreList, client.InNamespace(zoneNamespace))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list EventStores in namespace %q", zoneNamespace)
	}

	if len(eventStoreList.Items) == 0 {
		return nil, ctrlerrors.BlockedErrorf("no EventStore found in namespace %q", zoneNamespace)
	}

	return &eventStoreList.Items[0], nil
}
