// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"slices"
	"sort"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/index"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GetZone retrieves a Zone object by ObjectRef and ensures it is ready.
func GetZone(ctx context.Context, ref client.ObjectKey) (*adminv1.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zone := &adminv1.Zone{}
	err := c.Get(ctx, ref, zone)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("zone %q not found", ref.String())
		}
		return nil, errors.Wrapf(err, "failed to get zone %q", ref.String())
	}
	if err := condition.EnsureReady(zone); err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q is not ready", ref.String())
	}

	return zone, nil
}

// GetEventConfigForZone finds the EventConfig for a given zone name using the field index.
// Returns BlockedError if no EventConfig is found or if it is not ready.
func GetEventConfigForZone(ctx context.Context, zoneName string) (*eventv1.EventConfig, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	eventConfigList := &eventv1.EventConfigList{}
	err := c.List(ctx, eventConfigList,
		client.MatchingFields{index.EventConfigZoneIndex: zoneName})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list EventConfigs for zone %q", zoneName)
	}

	if len(eventConfigList.Items) == 0 {
		return nil, ctrlerrors.BlockedErrorf("no EventConfig found for zone %q", zoneName)
	}

	// Should be exactly 1 (1:1 zone-to-eventconfig), but be defensive
	if len(eventConfigList.Items) > 1 {
		slices.SortStableFunc(eventConfigList.Items, func(a, b eventv1.EventConfig) int {
			return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		})
		logger.Info("multiple EventConfigs found for zone, using first", "zone", zoneName, "count", len(eventConfigList.Items))
	}
	eventConfig := &eventConfigList.Items[0]
	logger.V(1).Info("Found EventConfig for zone", "zone", zoneName, "eventConfig", eventConfig.Name)

	if err := condition.EnsureReady(eventConfig); err != nil {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q for zone %q is not ready", eventConfig.Name, zoneName)
	}

	return eventConfig, nil
}

// GetEventStoreForZone resolves the Zone -> EventConfig -> EventStore chain.
// Returns the ready EventStore for a given zone name.
func GetEventStoreForZone(ctx context.Context, zoneName string) (*pubsubv1.EventStore, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventConfig, err := GetEventConfigForZone(ctx, zoneName)
	if err != nil {
		return nil, err
	}

	if eventConfig.Status.EventStore == nil {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q has no EventStore reference yet", eventConfig.Name)
	}

	eventStore := &pubsubv1.EventStore{}
	err = c.Get(ctx, eventConfig.Status.EventStore.K8s(), eventStore)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("EventStore %q not found", eventConfig.Status.EventStore.String())
		}
		return nil, errors.Wrapf(err, "failed to get EventStore %q", eventConfig.Status.EventStore.String())
	}

	if err := condition.EnsureReady(eventStore); err != nil {
		return nil, ctrlerrors.BlockedErrorf("EventStore %q is not ready", eventStore.Name)
	}

	return eventStore, nil
}

// FindActiveEventType finds the active EventType for a given event type string.
// Returns (found, eventType, error). If found is false, there is no active EventType.
func FindActiveEventType(ctx context.Context, eventType string) (bool, *eventv1.EventType, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventTypeList := &eventv1.EventTypeList{}
	if err := c.List(ctx, eventTypeList); err != nil {
		return false, nil, errors.Wrapf(err, "failed to list EventTypes for type %q", eventType)
	}

	// Filter to matching type and sort by creation timestamp (oldest first)
	var candidates []eventv1.EventType
	for _, et := range eventTypeList.Items {
		if et.Spec.Type == eventType && et.Status.Active {
			candidates = append(candidates, et)
		}
	}

	if len(candidates) == 0 {
		return false, nil, nil
	}

	// Sort by CreationTimestamp ascending and return the oldest active one
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreationTimestamp.Before(&candidates[j].CreationTimestamp)
	})

	activeET := &candidates[0]
	if err := condition.EnsureReady(activeET); err != nil {
		return false, activeET, ctrlerrors.BlockedErrorf("EventType %q is not ready", eventType)
	}

	return true, activeET, nil
}

// GetApplication retrieves an Application object by ObjectRef and ensures it is ready.
func GetApplication(ctx context.Context, ref types.ObjectRef) (*applicationapi.Application, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	application := &applicationapi.Application{}
	err := c.Get(ctx, ref.K8s(), application)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("application %q not found", ref.String())
		}
		return nil, errors.Wrapf(err, "failed to get application %q", ref.String())
	}
	if err := condition.EnsureReady(application); err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q is not ready", ref.String())
	}

	return application, nil
}

// FindCrossZoneSSESubscriptionZones lists all EventSubscriptions for a given event type
// and returns the unique zone ObjectRefs where cross-zone SSE subscriptions exist.
// A subscription is cross-zone if its zone differs from the exposure's zone,
// and is SSE if its delivery type is "ServerSentEvent".
func FindCrossZoneSSESubscriptionZones(ctx context.Context, eventType string, exposureZoneName string) ([]types.ObjectRef, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	subList := &eventv1.EventSubscriptionList{}
	if err := c.List(ctx, subList, client.MatchingLabels{
		eventv1.EventTypeLabelKey: labelutil.NormalizeLabelValue(eventType),
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to list EventSubscriptions for event type %q", eventType)
	}

	seen := make(map[string]bool)
	var zones []types.ObjectRef
	for _, sub := range subList.Items {
		if sub.Spec.EventType != eventType {
			continue
		}
		if sub.Spec.Delivery.Type != eventv1.DeliveryTypeServerSentEvent {
			continue
		}
		if sub.Spec.Zone.Name == exposureZoneName {
			continue
		}

		approvalCond := meta.FindStatusCondition(sub.GetConditions(), "ApprovalGranted")
		if approvalCond == nil || approvalCond.Status != metav1.ConditionTrue {
			logger.Info("Skipping subscription with missing approval", "subscription", sub.Name, "zone", sub.Spec.Zone.Name, "reason", approvalCond.Reason)
			continue
		}

		zoneName := sub.Spec.Zone.Name
		if !seen[zoneName] {
			seen[zoneName] = true
			zones = append(zones, sub.Spec.Zone)
		}
	}

	return zones, nil
}

// AnyOtherEventExposureExists checks if any other EventExposure (active or inactive)
// exists for the given event type, excluding the one with the given UID.
// This is used in the Delete handler to determine whether the Route should be preserved
// for a standby exposure to take over.
func AnyOtherEventExposureExists(ctx context.Context, eventType string, excludeUID k8stypes.UID) (bool, error) {
	candidates, err := FindEventExposures(ctx, eventType)
	if err != nil {
		return false, err
	}

	for _, exp := range candidates {
		if exp.UID == excludeUID {
			continue
		}
		if exp.Spec.EventType == eventType {
			return true, nil
		}
	}

	return false, nil
}

// FindEventExposures lists all EventExposures for a given event type, regardless of status.
func FindEventExposures(ctx context.Context, eventType string) ([]eventv1.EventExposure, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	exposureList := &eventv1.EventExposureList{}
	if err := c.List(ctx, exposureList, client.MatchingLabels{
		eventv1.EventTypeLabelKey: labelutil.NormalizeLabelValue(eventType),
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to list EventExposures for type %q", eventType)
	}

	var exposures []eventv1.EventExposure
	for _, exp := range exposureList.Items {
		if exp.Spec.EventType == eventType {
			exposures = append(exposures, exp)
		}
	}

	return exposures, nil
}

// FindActiveEventExposure finds the active EventExposure for a given event type.
// It should be used in combination with FindEventExposures to avoid multiple list calls.
// This function does not check if the exposure is ready! It only checks the Status.Active field.
func FindActiveEventExposure(exposures []eventv1.EventExposure) (bool, *eventv1.EventExposure, error) {
	if len(exposures) == 0 {
		return false, nil, nil
	}

	var candidates []eventv1.EventExposure
	for _, exp := range exposures {
		if exp.Status.Active {
			candidates = append(candidates, exp)
		}
	}

	if len(candidates) == 0 {
		return false, nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreationTimestamp.Before(&candidates[j].CreationTimestamp)
	})

	activeExp := &candidates[0]
	return true, activeExp, nil
}

func FindCrossZoneCallbackSubscriptions(ctx context.Context, eventType string, exposureZoneName string) ([]eventv1.EventSubscription, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	subList := &eventv1.EventSubscriptionList{}
	if err := c.List(ctx, subList, client.MatchingLabels{
		eventv1.EventTypeLabelKey: labelutil.NormalizeLabelValue(eventType),
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to list EventSubscriptions for event type %q", eventType)
	}

	var subs []eventv1.EventSubscription
	for _, sub := range subList.Items {
		if sub.Spec.EventType != eventType {
			continue
		}
		if sub.Spec.Delivery.Type != eventv1.DeliveryTypeCallback {
			continue
		}
		if sub.Spec.Zone.Name == exposureZoneName {
			continue
		}

		approvalCond := meta.FindStatusCondition(sub.GetConditions(), "ApprovalGranted")
		if approvalCond == nil || approvalCond.Status != metav1.ConditionTrue {
			logger.Info("Skipping subscription with missing approval", "subscription", sub.Name, "zone", sub.Spec.Zone.Name, "reason", approvalCond.Reason)
			continue
		}

		subs = append(subs, sub)
	}

	return subs, nil
}
