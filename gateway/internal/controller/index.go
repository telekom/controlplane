// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var IndexFieldSpecRoute = "spec.route"
var IndexFieldSpecRouteName = "spec.route.name"
var IndexFieldSpecRealm = "spec.realm"

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

	// Index the consumer by the realm it references
	filterRealmOnConsumer := func(obj client.Object) []string {
		consumer, ok := obj.(*gatewayv1.Consumer)
		if !ok {
			return nil
		}
		return []string{consumer.Spec.Realm.String()}
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &gatewayv1.Consumer{}, IndexFieldSpecRealm, filterRealmOnConsumer)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Consumer", "FieldIndex", IndexFieldSpecRealm)
		os.Exit(1)
	}

	// Index the route by the realm it references
	filterRealmOnRoute := func(obj client.Object) []string {
		route, ok := obj.(*gatewayv1.Route)
		if !ok {
			return nil
		}
		return []string{route.Spec.Realm.String()}
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &gatewayv1.Route{}, IndexFieldSpecRealm, filterRealmOnRoute)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Route", "FieldIndex", IndexFieldSpecRealm)
		os.Exit(1)
	}

}
