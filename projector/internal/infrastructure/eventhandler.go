// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SyncEventHandler wraps a standard controller-runtime EventHandler to
// intercept delete events and populate the DeleteCache with the last-known
// object state before the reconcile request is enqueued.
type SyncEventHandler struct {
	inner       handler.TypedEventHandler[client.Object, reconcile.Request]
	deleteCache *DeleteCache
}

// NewSyncEventHandler creates a SyncEventHandler that wraps
// EnqueueRequestForObject and stores deleted objects in the given cache.
func NewSyncEventHandler(deleteCache *DeleteCache) *SyncEventHandler {
	return &SyncEventHandler{
		inner:       &handler.EnqueueRequestForObject{},
		deleteCache: deleteCache,
	}
}

// Create delegates to the inner handler.
func (h *SyncEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.inner.Create(ctx, evt, q)
}

// Update delegates to the inner handler.
func (h *SyncEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.inner.Update(ctx, evt, q)
}

// Delete captures the last-known object state in the DeleteCache, then
// delegates to the inner handler to enqueue the reconcile request.
func (h *SyncEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.deleteCache.Store(evt.Object)
	h.inner.Delete(ctx, evt, q)
}

// Generic delegates to the inner handler.
func (h *SyncEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.inner.Generic(ctx, evt, q)
}
