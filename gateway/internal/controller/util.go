// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// mapRealmToObjects is a generic helper function that maps a Realm object to reconcile requests
// for objects that reference that realm. The listType parameter must be a pointer to a list type
// (like &gatewayv1.ConsumerList{} or &gatewayv1.RouteList{}).
func mapRealmToObjects(ctx context.Context, c client.Client, obj client.Object, listType client.ObjectList) []reconcile.Request {
	// ensure its actually a Realm
	realm, ok := obj.(*gatewayv1.Realm)
	if !ok {
		return nil
	}
	if realm.Labels == nil {
		return nil
	}

	listOpts := []client.ListOption{
		client.MatchingFields{
			IndexFieldSpecRealm: types.ObjectRefFromObject(realm).String(),
		},
		client.MatchingLabels{
			cconfig.EnvironmentLabelKey: realm.Labels[cconfig.EnvironmentLabelKey],
		},
	}

	if err := c.List(ctx, listType, listOpts...); err != nil {
		return nil
	}

	// Extract items from the list using reflection-like approach
	// We need to handle this differently since we can't use generics easily here
	switch list := listType.(type) {
	case *gatewayv1.ConsumerList:
		requests := make([]reconcile.Request, len(list.Items))
		for i, item := range list.Items {
			requests[i] = reconcile.Request{NamespacedName: client.ObjectKey{Name: item.Name, Namespace: item.Namespace}}
		}
		return requests
	case *gatewayv1.RouteList:
		requests := make([]reconcile.Request, len(list.Items))
		for i, item := range list.Items {
			requests[i] = reconcile.Request{NamespacedName: client.ObjectKey{Name: item.Name, Namespace: item.Namespace}}
		}
		return requests
	default:
		return nil
	}
}
