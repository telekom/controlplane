// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/types"
)

func cleanupState(ctx context.Context, c ScopedClient, listOpts []client.ListOption, objectList types.ObjectList, desiredStateSet map[client.ObjectKey]bool) (int, error) {
	deleted := 0
	err := c.List(ctx, objectList, listOpts...)
	if err != nil {
		return deleted, errors.Wrap(err, "failed to list objects")
	}
	logger := log.FromContext(ctx)
	logger.V(1).Info("cleanup state", "found", len(objectList.GetItems()), "desired", len(desiredStateSet))

	for _, object := range objectList.GetItems() {
		if _, ok := desiredStateSet[client.ObjectKey{Name: object.GetName(), Namespace: object.GetNamespace()}]; ok {
			continue
		}
		logger.V(1).Info("deleting object", "name", object.GetName(), "namespace", object.GetNamespace())
		delErr := c.Delete(ctx, object, &client.DeleteOptions{})
		if delErr != nil && !apierrors.IsNotFound(delErr) {
			return deleted, errors.Wrapf(delErr, "failed to delete object %s", object.GetName())
		}
		deleted++
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
	logger := log.FromContext(ctx)

	// Ensure that we have a GVK for a List type
	if !strings.HasSuffix(gvk.Kind, "List") {
		gvk.Kind += "List"
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

	logger.V(1).Info("cleanup state", "found", len(objectList.GetItems()), "desired", len(desiredStateSet))

	for _, object := range objectList.GetItems() {
		if _, ok := desiredStateSet[client.ObjectKey{Name: object.GetName(), Namespace: object.GetNamespace()}]; ok {
			continue
		}
		logger.V(1).Info("deleting object", "name", object.GetName(), "namespace", object.GetNamespace())
		delErr := c.Delete(ctx, object, &client.DeleteOptions{})
		if delErr != nil && !apierrors.IsNotFound(delErr) {
			return deleted, objectList, errors.Wrapf(delErr, "failed to delete object %s", object.GetName())
		}
		deleted++
	}
	if deleted > 0 {
		err = c.List(ctx, objectList, listOpts...)
		if err != nil {
			return deleted, objectList, errors.Wrap(err, "failed to list updated objects")
		}
	}

	return deleted, objectList, nil
}
