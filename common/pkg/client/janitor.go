// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type JanitorClient interface {
	ScopedClient
	// Cleanup cleans up all objects of type objectList that were not created or updated during the current reconciliation.
	Cleanup(ctx context.Context, objectList types.ObjectList, listOpts []client.ListOption) (deleted int, err error)
	// Wrap wraps the given function with a cleanup of all objects that were not created or updated during the current reconciliation.
	Wrap(ctx context.Context, objectList types.ObjectList, listOpts []client.ListOption, f func(ScopedClient) bool) (deleted int, err error)
	// CleanupAll cleans up all objects that were not created or updated during the current reconciliation.
	CleanupAll(ctx context.Context, listOpts []client.ListOption) (int, error)
	// AddKnownTypeToState ensures that the given object type is tracked in the state.
	// This means that the janitor will clean up all objects of this type that were not created or updated during the current reconciliation.
	AddKnownTypeToState(obj types.Object)
}

type janitorClient struct {
	ScopedClient
	state map[schema.GroupVersionKind]map[client.ObjectKey]bool
}

func NewJanitorClient(c ScopedClient) JanitorClient {
	return &janitorClient{
		ScopedClient: c,
		state:        make(map[schema.GroupVersionKind]map[client.ObjectKey]bool),
	}
}

func (c *janitorClient) Reset() {
	c.ScopedClient.Reset()
	c.state = make(map[schema.GroupVersionKind]map[client.ObjectKey]bool)
}

func (c *janitorClient) AddKnownTypeToState(obj types.Object) {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		panic(errors.Wrap(err, "failed to get GVK for object"))
	}
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")

	if _, ok := c.state[gvk]; !ok {
		c.state[gvk] = make(map[client.ObjectKey]bool)
	}
}

func (c *janitorClient) Cleanup(ctx context.Context, objectList types.ObjectList, listOpts []client.ListOption) (int, error) {
	gvk, err := apiutil.GVKForObject(objectList, c.Scheme())
	if err != nil {
		return -1, errors.Wrap(err, "failed to get GVK")
	}
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")

	n, err := cleanupState(ctx, c, listOpts, objectList, c.state[gvk])
	if err != nil {
		return n, errors.Wrapf(err, "failed to cleanup state for %s", gvk.String())
	}
	delete(c.state, gvk)
	return n, nil
}

func (c *janitorClient) Wrap(ctx context.Context, objectList types.ObjectList, listOpts []client.ListOption, f func(ScopedClient) bool) (int, error) {
	gvk := objectList.GetObjectKind().GroupVersionKind()
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")

	if f(c) {
		return c.Cleanup(ctx, objectList, listOpts)
	}

	return -1, errors.New("aborted by user")
}

func (c *janitorClient) CreateOrUpdate(ctx context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "failed to get GVK for object")
	}

	res, err := c.ScopedClient.CreateOrUpdate(ctx, obj, mutate)
	if err != nil {
		return res, errors.Wrap(err, "failed to create or update object")
	}

	if _, ok := c.state[gvk]; !ok {
		c.state[gvk] = make(map[client.ObjectKey]bool)
	}
	c.state[gvk][client.ObjectKeyFromObject(obj)] = true

	return res, nil
}

func (c *janitorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return errors.Wrap(err, "failed to get GVK for object")
	}

	err = c.ScopedClient.Delete(ctx, obj, opts...)
	if err != nil {
		return err
	}

	if gvk, ok := c.state[gvk]; ok {
		delete(gvk, client.ObjectKeyFromObject(obj))
	}

	return nil
}

func (c *janitorClient) CleanupAll(ctx context.Context, listOpts []client.ListOption) (int, error) {
	log := log.FromContext(ctx)
	deleted := 0
	for gvk, keys := range c.state {
		log.V(1).Info("Cleaning up state", "gvk", gvk)

		n, _, err := cleanupStateUnstructured(ctx, c, listOpts, gvk, keys)
		if err != nil {
			return deleted, errors.Wrapf(err, "failed to cleanup state for %s", gvk.String())
		}
		delete(c.state, gvk)
		deleted += n
	}

	return deleted, nil
}
