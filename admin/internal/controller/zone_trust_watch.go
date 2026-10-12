// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	zone_handler "github.com/telekom/controlplane/admin/internal/handler/zone"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/providerca"
	"github.com/telekom/controlplane/common/pkg/types"
)

// ProviderCASourceIndex indexes Zones by "<namespace>/<configmap>" of every referenced CA source.
const ProviderCASourceIndex = "spec.providerCASources"

func providerCASourceKey(namespace, name string) string { return namespace + "/" + name }

func indexZoneProviderCASources(obj client.Object) []string {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}
	keys := map[string]struct{}{}
	add := func(t *types.ProviderTLS) {
		if ref := t.Ref(); ref != nil {
			keys[providerCASourceKey(zone.Namespace, ref.Name)] = struct{}{}
		}
	}
	for i := range zone.Spec.IdentityProviders {
		add(zone.Spec.IdentityProviders[i].TLS)
	}
	for i := range zone.Spec.Gateways {
		add(zone.Spec.Gateways[i].TLS)
	}
	return slices.Sorted(maps.Keys(keys))
}

// providerCAValidityLabels are the labels that copy/source validation depends on.
var providerCAValidityLabels = []string{
	types.ProviderCALabelKey,
	config.EnvironmentLabelKey,
	config.BuildLabelKey(zoneLabelName),
	config.DomainLabelKey,
}

func validityLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for _, k := range providerCAValidityLabels {
		if v, ok := labels[k]; ok {
			out[k] = v
		}
	}
	return out
}

func adminAnnotations(annotations map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range annotations {
		if strings.HasPrefix(k, zone_handler.AnnotationPrefix) {
			out[k] = v
		}
	}
	return out
}

// ProviderCAConfigMapPredicate passes updates that change anything source/copy validation depends
// on: data, binaryData, the provider-ca/environment/zone/domain labels, Admin provenance annotations,
// ownerReferences or immutability. Unrelated metadata changes are ignored. Create and delete events
// always pass, including deletes caused by removing the provider-ca label.
var ProviderCAConfigMapPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldCM, okOld := e.ObjectOld.(*corev1.ConfigMap)
		newCM, okNew := e.ObjectNew.(*corev1.ConfigMap)
		if !okOld || !okNew {
			return true
		}
		return !equality.Semantic.DeepEqual(oldCM.Data, newCM.Data) ||
			!equality.Semantic.DeepEqual(oldCM.BinaryData, newCM.BinaryData) ||
			!equality.Semantic.DeepEqual(validityLabels(oldCM.Labels), validityLabels(newCM.Labels)) ||
			!equality.Semantic.DeepEqual(adminAnnotations(oldCM.Annotations), adminAnnotations(newCM.Annotations)) ||
			!equality.Semantic.DeepEqual(oldCM.OwnerReferences, newCM.OwnerReferences) ||
			!equality.Semantic.DeepEqual(oldCM.Immutable, newCM.Immutable)
	},
}

// mapConfigMapToZones maps a source through the namespace-qualified source index and a durable
// copy through its verified origin metadata to live Zones. The label is never used for identification.
// Copies with invalid provenance rely on other events or periodic reconciliation for detection.
func (r *ZoneReconciler) mapConfigMapToZones(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logr.FromContextOrDiscard(ctx)
	requests := map[client.ObjectKey]struct{}{}

	zones := &adminv1.ZoneList{}
	if err := r.List(ctx, zones, client.MatchingFields{ProviderCASourceIndex: providerCASourceKey(obj.GetNamespace(), obj.GetName())}); err != nil {
		log.Error(err, "Failed to list zones for provider CA source", "configMap", client.ObjectKeyFromObject(obj))
	} else {
		for i := range zones.Items {
			requests[client.ObjectKeyFromObject(&zones.Items[i])] = struct{}{}
		}
	}

	if origin, ok := providerca.ProviderCAOriginFromObject(obj); ok {
		zone := &adminv1.Zone{}
		key := client.ObjectKey{Namespace: origin.ZoneNamespace, Name: origin.ZoneName}
		if err := r.Get(ctx, key, zone); err != nil {
			log.V(1).Info("Zone of provider CA copy not found", "configMap", client.ObjectKeyFromObject(obj), "error", err.Error())
		} else if string(zone.UID) == origin.ZoneUID {
			requests[key] = struct{}{}
		}
	}

	result := make([]reconcile.Request, 0, len(requests))
	for key := range requests {
		result = append(result, reconcile.Request{NamespacedName: key})
	}
	slices.SortFunc(result, func(a, b reconcile.Request) int {
		if a.Namespace != b.Namespace {
			if a.Namespace < b.Namespace {
				return -1
			}
			return 1
		}
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return result
}
