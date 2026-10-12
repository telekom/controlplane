// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

const (
	// IndexFieldRouteGateway indexes Routes by namespace/name of their Gateway.
	IndexFieldRouteGateway = "spec.gatewayRef.namespacedName"
	// IndexFieldConsumerGateway indexes Consumers by namespace/name of their Gateway.
	IndexFieldConsumerGateway = "spec.gateway.namespacedName"
)

func refKey(ref ctypes.ObjectRef) string {
	return ref.Namespace + "/" + ref.Name
}

func registerGatewayDependentIndices(ctx context.Context, mgr ctrl.Manager) error {
	idx := mgr.GetFieldIndexer()
	if err := idx.IndexField(ctx, &gatewayv1.Route{}, IndexFieldRouteGateway, func(o client.Object) []string {
		r, ok := o.(*gatewayv1.Route)
		if !ok {
			return nil
		}
		return []string{refKey(r.Spec.GatewayRef)}
	}); err != nil {
		return err
	}
	return idx.IndexField(ctx, &gatewayv1.Consumer{}, IndexFieldConsumerGateway, func(o client.Object) []string {
		c, ok := o.(*gatewayv1.Consumer)
		if !ok {
			return nil
		}
		return []string{refKey(c.Spec.Gateway)}
	})
}

// gatewayFanout maps Gateway changes to their Routes and Consumers, including terminating ones.
type gatewayFanout struct {
	client client.Client
}

// MapGatewayToRoutes enqueues Routes of the Gateway.
func (t *gatewayFanout) MapGatewayToRoutes(ctx context.Context, obj client.Object) []reconcile.Request {
	return t.routesFor(ctx, obj.GetNamespace(), obj.GetName(), obj.GetLabels())
}

// MapGatewayToConsumers enqueues Consumers of the Gateway.
func (t *gatewayFanout) MapGatewayToConsumers(ctx context.Context, obj client.Object) []reconcile.Request {
	return t.consumersFor(ctx, obj.GetNamespace(), obj.GetName(), obj.GetLabels())
}

func envOpts(labels map[string]string) []client.ListOption {
	if env, ok := labels[cconfig.EnvironmentLabelKey]; ok && env != "" {
		return []client.ListOption{client.MatchingLabels{cconfig.EnvironmentLabelKey: env}}
	}
	return nil
}

func (t *gatewayFanout) routesFor(ctx context.Context, ns, name string, labels map[string]string) []reconcile.Request {
	list := &gatewayv1.RouteList{}
	opts := append([]client.ListOption{client.MatchingFields{IndexFieldRouteGateway: ns + "/" + name}}, envOpts(labels)...)
	if err := t.client.List(ctx, list, opts...); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "Failed to list routes for gateway", "gateway", ns+"/"+name)
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return sortedUnique(reqs)
}

func (t *gatewayFanout) consumersFor(ctx context.Context, ns, name string, labels map[string]string) []reconcile.Request {
	list := &gatewayv1.ConsumerList{}
	opts := append([]client.ListOption{client.MatchingFields{IndexFieldConsumerGateway: ns + "/" + name}}, envOpts(labels)...)
	if err := t.client.List(ctx, list, opts...); err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "Failed to list consumers for gateway", "gateway", ns+"/"+name)
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return sortedUnique(reqs)
}

func sortedUnique(reqs []reconcile.Request) []reconcile.Request {
	slices.SortFunc(reqs, func(a, b reconcile.Request) int {
		return strings.Compare(a.String(), b.String())
	})
	return slices.Compact(reqs)
}

// gatewayChanged admits Gateway create/delete and spec or label changes, but not status writes.
var gatewayChanged = predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{})
