// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
)

// IndexFieldCAConfigMapName indexes Zones by the names of the ConfigMaps
// referenced as CA sources. The references are local to the Zone namespace.
const IndexFieldCAConfigMapName = ".spec.tls.configMapRef.name"

func indexCAConfigMapNames(obj client.Object) []string {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}
	var names []string
	for _, ref := range zone.Spec.CARefs() {
		if len(names) == 0 || names[len(names)-1] != ref.Name {
			names = append(names, ref.Name)
		}
	}
	return names
}

// mapConfigMapToZones maps a CA source ConfigMap to the Zones referencing it,
// and a managed CA bundle to its owning Zone.
func (r *ZoneReconciler) mapConfigMapToZones(ctx context.Context, obj client.Object) []reconcile.Request {
	var requests []reconcile.Request
	if obj.GetLabels()[cconfig.DomainLabelKey] == domainName {
		requests = r.mapSubResourceToZone(ctx, obj)
	}

	zones := &adminv1.ZoneList{}
	err := r.List(ctx, zones, client.InNamespace(obj.GetNamespace()), client.MatchingFields{IndexFieldCAConfigMapName: obj.GetName()})
	if err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "Failed to list zones referencing ConfigMap", "configMap", client.ObjectKeyFromObject(obj).String())
		return requests
	}
	for i := range zones.Items {
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&zones.Items[i])}
		if len(requests) == 0 || requests[0] != req {
			requests = append(requests, req)
		}
	}
	return requests
}
