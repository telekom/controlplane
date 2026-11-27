// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package infrastructure provides shared infrastructure components for the
// projector, including caching, event handling, and FK resolution.
package infrastructure

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteCache is a lightweight concurrent map that stores the last-known
// object state from informer delete events, keyed by NamespacedName.
// It is consumed by the reconciler when a Get returns NotFound.
type DeleteCache struct {
	cache sync.Map // map[types.NamespacedName]client.Object
}

// Store saves the last-known state of obj, keyed by its namespace and name.
func (c *DeleteCache) Store(obj client.Object) {
	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
	c.cache.Store(key, obj)
}

// LoadAndDelete atomically loads and removes the cached object for key.
// Returns nil if no entry exists.
func (c *DeleteCache) LoadAndDelete(key types.NamespacedName) client.Object {
	val, ok := c.cache.LoadAndDelete(key)
	if !ok {
		return nil
	}
	obj, _ := val.(client.Object)
	return obj
}

// Load returns the cached object for key without removing it, or nil if no
// entry exists. Pair it with Delete once the delete has been fully processed,
// so that a failed delete can be retried with the same last-known state.
func (c *DeleteCache) Load(key types.NamespacedName) client.Object {
	val, ok := c.cache.Load(key)
	if !ok {
		return nil
	}
	obj, _ := val.(client.Object)
	return obj
}

// Delete removes the cached object for key.
func (c *DeleteCache) Delete(key types.NamespacedName) {
	c.cache.Delete(key)
}
