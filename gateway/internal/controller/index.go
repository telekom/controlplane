// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"

	"github.com/telekom/controlplane/common/pkg/controller/index"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var IndexFieldSpecRoute = "spec.route"
var IndexFieldSpecRouteName = "spec.route.name"

func RegisterIndecesOrDie(ctx context.Context, mgr ctrl.Manager) {
	// Index the consumeRoute by the route it references
	filterRouteOnConsumeRoute := func(obj client.Object) []string {
		consumeRoute, ok := obj.(*gatewayv1.ConsumeRoute)
		if !ok {
			return nil
		}
		return []string{consumeRoute.Spec.Route.String()}
	}

	err := mgr.GetFieldIndexer().IndexField(ctx, &gatewayv1.ConsumeRoute{}, IndexFieldSpecRoute, filterRouteOnConsumeRoute)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for ConsumeRoute", "FieldIndex", IndexFieldSpecRoute)
		os.Exit(1)
	}

	// Index the consumeRoute by the route.name it references
	filterRouteNameOnConsumeRoute := func(obj client.Object) []string {
		consumeRoute, ok := obj.(*gatewayv1.ConsumeRoute)
		if !ok {
			return nil
		}
		return []string{consumeRoute.Spec.Route.Name}
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &gatewayv1.ConsumeRoute{}, IndexFieldSpecRouteName, filterRouteNameOnConsumeRoute)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for ConsumeRoute", "FieldIndex", IndexFieldSpecRouteName)
		os.Exit(1)
	}

	// Index the routeListener by the route it references
	filterRouteOnRouteListener := func(obj client.Object) []string {
		routeListener, ok := obj.(*gatewayv1.RouteListener)
		if !ok {
			return nil
		}
		return []string{routeListener.Spec.Route.String()}
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &gatewayv1.RouteListener{}, IndexFieldSpecRoute, filterRouteOnRouteListener)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for RouteListener", "FieldIndex", IndexFieldSpecRoute)
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gatewayv1.Route{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}
}
