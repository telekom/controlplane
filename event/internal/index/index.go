// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"
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
	EventConfigZoneIndex = ".spec.zone.name"
)

func RegisterIndicesOrDie(ctx context.Context, mgr ctrl.Manager) {
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
