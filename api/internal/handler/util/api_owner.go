// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// FindApiByBasePath returns the Api with the given base path in the given namespace,
// or nil if there is none.
func FindApiByBasePath(ctx context.Context, c client.Reader, namespace, basePath string) (*apiv1.Api, error) {
	list := &apiv1.ApiList{}
	err := c.List(ctx, list,
		// Namespace isolation is intended: every environment owns its own namespace.
		client.InNamespace(namespace),
		client.MatchingLabels{apiv1.BasePathLabelKey: labelutil.NormalizeLabelValue(basePath)},
	)
	if err != nil {
		return nil, fmt.Errorf("listing Apis for base path %q: %w", basePath, err)
	}

	items := slices.DeleteFunc(list.Items, func(a apiv1.Api) bool { return a.Spec.BasePath != basePath })
	if len(items) == 0 {
		return nil, nil
	}

	// Cached list order is not stable, so pick deterministically.
	slices.SortFunc(items, func(a, b apiv1.Api) int { return cmp.Compare(a.Name, b.Name) })
	return &items[0], nil
}

// EnsureApiOwner makes the Api that has the given base path (if it exists in the namespace of obj)
// an owner of obj, so that deleting the Api garbage-collects obj. The reverse does not hold:
// deleting obj never affects the Api. Returns the Api, or nil if none exists (yet).
//
// A successful metadata update refreshes obj from the server, so callers must set
// status conditions after calling this function.
func EnsureApiOwner(ctx context.Context, c client.Client, obj client.Object, basePath string) (*apiv1.Api, error) {
	owner, err := FindApiByBasePath(ctx, c, obj.GetNamespace(), basePath)
	if err != nil || owner == nil {
		return nil, err
	}

	if hasOwner(obj, owner.UID) {
		return owner, nil
	}

	original, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return nil, fmt.Errorf("copying %T: not a client.Object", obj)
	}
	patchBase := client.MergeFrom(original)
	if err := controllerutil.SetOwnerReference(owner, obj, c.Scheme()); err != nil {
		return nil, fmt.Errorf("setting owner reference to Api %q: %w", owner.Name, err)
	}
	if err := c.Patch(ctx, obj, patchBase); err != nil {
		return nil, fmt.Errorf("persisting owner reference to Api %q: %w", owner.Name, err)
	}
	return owner, nil
}

func hasOwner(obj client.Object, uid types.UID) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == uid {
			return true
		}
	}
	return false
}
