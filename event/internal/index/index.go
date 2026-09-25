// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"
	"fmt"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

const (
	// EventConfigZoneIndex indexes EventConfig by spec.zone.name for efficient lookups.
	EventConfigZoneIndex                    = ".spec.zone.name"
	EventSubscriptionZoneIndex              = ".spec.zone.ref"
	EventExposureZoneIndex                  = ".spec.zone.ref"
	EventSubscriptionCallbackEventTypeIndex = ".spec.callback.eventType"
)

// RegisterEventSubscriptionMapperIndices registers the cache fields used for
// routing EventConfig changes to subscriptions.
func RegisterEventSubscriptionMapperIndices(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(ctx, &eventv1.EventSubscription{}, EventSubscriptionZoneIndex, func(obj client.Object) []string {
		sub, ok := obj.(*eventv1.EventSubscription)
		if !ok {
			return nil
		}
		zone := sub.Spec.Zone
		if zone.Name == "" || zone.Namespace == "" {
			return nil
		}
		return []string{zone.String()}
	}); err != nil {
		return fmt.Errorf("index EventSubscription zone: %w", err)
	}
	if err := indexer.IndexField(ctx, &eventv1.EventExposure{}, EventExposureZoneIndex, func(obj client.Object) []string {
		exposure, ok := obj.(*eventv1.EventExposure)
		if !ok {
			return nil
		}
		zone := exposure.Spec.Zone
		if zone.Name == "" || zone.Namespace == "" {
			return nil
		}
		return []string{zone.String()}
	}); err != nil {
		return fmt.Errorf("index EventExposure zone: %w", err)
	}
	if err := indexer.IndexField(ctx, &eventv1.EventSubscription{}, EventSubscriptionCallbackEventTypeIndex, func(obj client.Object) []string {
		sub, ok := obj.(*eventv1.EventSubscription)
		if !ok {
			return nil
		}
		if sub.Spec.Delivery.Type != eventv1.DeliveryTypeCallback || sub.Spec.EventType == "" {
			return nil
		}
		return []string{sub.Spec.EventType}
	}); err != nil {
		return fmt.Errorf("index EventSubscription callback event type: %w", err)
	}
	return nil
}

func RegisterIndicesOrDie(ctx context.Context, mgr ctrl.Manager) {
	if err := RegisterEventSubscriptionMapperIndices(ctx, mgr.GetFieldIndexer()); err != nil {
		ctrl.Log.Error(err, "unable to create EventSubscription mapper field indices")
		os.Exit(1)
	}
	indexEventConfigByZone := func(obj client.Object) []string {
		ec, ok := obj.(*eventv1.EventConfig)
		if !ok {
			return nil
		}
		if ec.Spec.Zone.Name == "" {
			return nil
		}
		return []string{ec.Spec.Zone.Name}
	}
	err := mgr.GetFieldIndexer().IndexField(ctx, &eventv1.EventConfig{}, EventConfigZoneIndex, indexEventConfigByZone)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for EventConfig", "FieldIndex", EventConfigZoneIndex)
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &pubsubv1.Subscriber{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &pubsubv1.EventStore{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &approvalapi.ApprovalRequest{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &identityv1.Client{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gatewayv1.Route{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}
}
