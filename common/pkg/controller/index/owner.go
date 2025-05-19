// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ControllerIndexKey = ".metadata.controller"
)

// SetOwnerIndex sets the owner index for the given object.
func SetOwnerIndex(ctx context.Context, indexer client.FieldIndexer, ownedObj client.Object) error {
	filterFunc := func(obj client.Object) []string {
		owner := metav1.GetControllerOf(obj)
		if owner == nil {
			return nil
		}

		return []string{string(owner.UID)}
	}

	return indexer.IndexField(ctx, ownedObj, ControllerIndexKey, filterFunc)
}

// SetOwnerIndexForOwner sets the owner index for the given object but only for the given owner GVK.
func SetOwnerIndexForOwner(ctx context.Context, indexer client.FieldIndexer, ownerGVK schema.GroupVersionKind, ownedObj client.Object) error {
	ownerKind := ownerGVK.Kind
	ownerApiVersion := ownerGVK.GroupVersion().String()

	filterFunc := func(obj client.Object) []string {
		owner := metav1.GetControllerOf(obj)
		if owner == nil {
			return nil
		}

		if owner.Kind != ownerKind || owner.APIVersion != ownerApiVersion {
			return nil
		}

		return []string{string(owner.UID)}
	}

	return indexer.IndexField(ctx, ownedObj, ControllerIndexKey, filterFunc)
}
