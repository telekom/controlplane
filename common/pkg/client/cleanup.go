// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func cleanupState(ctx context.Context, c ScopedClient, listOpts []client.ListOption, objectList types.ObjectList, desiredStateSet map[client.ObjectKey]bool) (int, error) {
	deleted := 0
	err := c.List(ctx, objectList, listOpts...)
	if err != nil {
		return deleted, errors.Wrap(err, "failed to list objects")
	}
	log := log.FromContext(ctx)
	log.V(1).Info("cleanup state", "found", len(objectList.GetItems()), "desired", len(desiredStateSet))

	for _, object := range objectList.GetItems() {
		if _, ok := desiredStateSet[client.ObjectKey{Name: object.GetName(), Namespace: object.GetNamespace()}]; !ok {
			log.V(1).Info("deleting object", "name", object.GetName(), "namespace", object.GetNamespace())
			err := c.Delete(ctx, object, &client.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				// If we have some internal error, we return the error and the result
				return deleted, errors.Wrapf(err, "failed to delete object %s", object.GetName())
			}
			deleted++
			continue
		}
	}
	if deleted > 0 {
		err = c.List(ctx, objectList, listOpts...)
		if err != nil {
			return deleted, errors.Wrap(err, "failed to list updated objects")
		}
	}

	return deleted, nil
}

func cleanupStateUnstructured(ctx context.Context, c ScopedClient, listOpts []client.ListOption, gvk schema.GroupVersionKind, desiredStateSet map[client.ObjectKey]bool) (int, types.ObjectList, error) {
	log := log.FromContext(ctx)

	// Ensure that we have a GVK for a List type
	if !strings.HasSuffix(gvk.Kind, "List") {
		gvk.Kind = gvk.Kind + "List"
	}

	// Get an instance of the List type
	o, err := c.Scheme().New(gvk)
	if err != nil {
		return 0, nil, errors.Wrap(err, "unknown type: "+gvk.String())
	}

	// We need to cast it to ObjectList
	objectList, ok := o.(types.ObjectList)
	if !ok {
		return 0, nil, errors.New("object is not a valid list")
	}

	deleted := 0
	err = c.List(ctx, objectList, listOpts...)
	if err != nil {
		return deleted, objectList, errors.Wrap(err, "failed to list objects")
	}

	log.V(1).Info("cleanup state", "found", len(objectList.GetItems()), "desired", len(desiredStateSet))

	for _, object := range objectList.GetItems() {
		if _, ok := desiredStateSet[client.ObjectKey{Name: object.GetName(), Namespace: object.GetNamespace()}]; !ok {
			log.V(1).Info("deleting object", "name", object.GetName(), "namespace", object.GetNamespace())
			err := c.Delete(ctx, object, &client.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				// If we have some internal error, we return the error and the result
				return deleted, objectList, errors.Wrapf(err, "failed to delete object %s", object.GetName())
			}
			deleted++
			continue
		}
	}
	if deleted > 0 {
		err = c.List(ctx, objectList, listOpts...)
		if err != nil {
			return deleted, objectList, errors.Wrap(err, "failed to list updated objects")
		}
	}

	return deleted, objectList, nil
}
