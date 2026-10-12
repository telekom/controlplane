// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"
)

// ProviderCAConfigMapSelector selects ConfigMaps labeled cp.ei.telekom.de/provider-ca=true.
func ProviderCAConfigMapSelector() labels.Selector {
	return labels.SelectorFromSet(labels.Set{types.ProviderCALabelKey: types.ProviderCALabelValue})
}

// ProviderCACacheOptions returns cache options with the given per-GVK entries plus a
// ConfigMap label selector. No DefaultLabelSelector is set. Provider CA resolution reads
// through this cache, so a referenced ConfigMap without the label is reported as not found.
// The selector narrows informer list/watch traffic only, not RBAC.
func ProviderCACacheOptions(existing map[client.Object]cache.ByObject) cache.Options {
	byObject := make(map[client.Object]cache.ByObject, len(existing)+1)
	for obj, entry := range existing {
		if _, isConfigMap := obj.(*corev1.ConfigMap); isConfigMap {
			continue
		}
		byObject[obj] = entry
	}
	byObject[&corev1.ConfigMap{}] = cache.ByObject{Label: ProviderCAConfigMapSelector()}
	return cache.Options{ByObject: byObject}
}
