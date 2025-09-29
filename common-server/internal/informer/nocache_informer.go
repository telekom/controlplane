// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// NoCacheInformer implements the Informer pattern without keeping a local cache.
// It flushes events directly to the event handler, which is useful for resources
// that are too large to store efficiently in memory.
type NoCacheInformer struct {
	ctx              context.Context
	gvr              schema.GroupVersionResource
	name             string
	k8sClient        dynamic.Interface
	eventHandler     EventHandler
	log              logr.Logger
	bufferSize       int64        // Maximum number of items to retrieve per list call
	queue            chan event   // Channel for event processing
	initDone         *atomic.Bool // Tracks if initial list is complete
	currentlyLoading *atomic.Bool // Prevents concurrent reloads
	resourceVersion  string       // Current resource version for watch operations
	workerCount      int          // Number of worker goroutines for event processing

	watcher       watch.Interface    // The Kubernetes watch client
	watcherCancel context.CancelFunc // Function to cancel the watcher context
	cancel        context.CancelFunc // Function to cancel the informer context

	resyncPeriod time.Duration // How often to perform a full resync

	mutex sync.Mutex // Protects access to shared fields
}

// NoCacheInformerOptions provides configurable options for the NoCacheInformer
type NoCacheInformerOptions struct {
	BufferSize   int64
	QueueSize    int
	WorkerCount  int
	ResyncPeriod time.Duration
}

// DefaultNoCacheInformerOptions returns the default options for a NoCacheInformer
func DefaultNoCacheInformerOptions() NoCacheInformerOptions {
	return NoCacheInformerOptions{
		BufferSize:   1000,
		QueueSize:    200,
		WorkerCount:  runtime.NumCPU(),
		ResyncPeriod: 5 * time.Minute,
	}
}

// [experimental] NewNoCache creates a new informer that does not use a local cache.
func NewNoCache(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler) *NoCacheInformer {
	return NewNoCacheWithOptions(ctx, gvr, k8sClient, eventHandler, DefaultNoCacheInformerOptions())
}

func NewNoCacheWithOptions(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler, options NoCacheInformerOptions) *NoCacheInformer {
	log := logr.FromContextOrDiscard(ctx)
	ctxWithCancel, cancel := context.WithCancel(ctx)
	name := fmt.Sprintf("NoCacheInformer:%s/%s", strings.ToLower(gvr.Group), strings.ToLower(gvr.Resource))
	return &NoCacheInformer{
		ctx:              ctxWithCancel,
		cancel:           cancel,
		gvr:              gvr,
		k8sClient:        k8sClient,
		eventHandler:     eventHandler,
		log:              log.WithName(name),
		name:             name,
		bufferSize:       options.BufferSize,
		queue:            make(chan event, options.QueueSize),
		initDone:         &atomic.Bool{},
		currentlyLoading: &atomic.Bool{},
		workerCount:      options.WorkerCount,
		resyncPeriod:     options.ResyncPeriod,
	}
}

type event struct {
	typ string
	obj *unstructured.Unstructured
}

func (i *NoCacheInformer) handlerLoop(ctx context.Context, inChan chan event) {
	for {
		select {
		case e := <-inChan:
			start := time.Now()
			var err error

			switch e.typ {
			case "ADDED":
				err = i.eventHandler.OnCreate(ctx, e.obj)
			case "MODIFIED":
				err = i.eventHandler.OnUpdate(ctx, e.obj)
			case "DELETED":
				err = i.eventHandler.OnDelete(ctx, e.obj)
			default:
				i.log.Info("Unknown event type", "type", e.typ)
			}

			if err != nil {
				i.log.Error(err, "Failed to handle event", "type", e.typ, "name", e.obj.GetName())
				counter.WithLabelValues(i.name, e.typ, "1").Inc()
			} else {
				counter.WithLabelValues(i.name, e.typ, "0").Inc()
			}

			// Record how long the event was being processed
			queueWaitTime.WithLabelValues(i.name).Observe(time.Since(start).Seconds())
			queueSize.WithLabelValues(i.name).Dec()

		case <-ctx.Done():
			i.log.Info("Handler loop stopped")
			return
		}
	}
}

func (i *NoCacheInformer) list(ctx context.Context, inChan chan event) error {
	var continueToken string

	ctx, cancel := context.WithTimeout(ctx, i.resyncPeriod*9/10)
	defer cancel()

	i.log.Info("Listing resources")
	listOperations.WithLabelValues(i.name).Inc()

	for {
		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context cancelled while listing resources")
		}

		list, err := i.k8sClient.Resource(i.gvr).List(ctx, metav1.ListOptions{
			Limit:    i.bufferSize,
			Continue: continueToken,
		})
		if err != nil {
			return errors.Wrap(err, "failed to list resources")
		}

		i.log.Info("Listed resources", "count", len(list.Items))

		// Process items in batches to avoid CPU spikes
		batchSize := 100
		for j := 0; j < len(list.Items); j += batchSize {
			end := min(j+batchSize, len(list.Items))

			// Process this batch
			for _, item := range list.Items[j:end] {
				inChan <- event{
					typ: "ADDED",
					obj: &item,
				}
				queueSize.WithLabelValues(i.name).Inc()
			}

			// Give the system a small breather between batches
			if end < len(list.Items) && len(list.Items) > batchSize {
				time.Sleep(time.Millisecond * 50)
			}
		}

		i.resourceVersion = list.GetResourceVersion()
		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}
	i.initDone.Store(true)

	return nil
}

func (i *NoCacheInformer) watchLoop(ctx context.Context, watcher watch.Interface) {
	for {
		select {
		case e := <-watcher.ResultChan():
			watchLoopIterations.WithLabelValues(i.name).Inc()
			start := time.Now()

			i.log.V(1).Info("Received event", "type", e.Type)
			switch e.Type {
			case watch.Error:
				status, ok := e.Object.(*metav1.Status)
				if !ok {
					counter.WithLabelValues(i.name, "ERROR", "unknown").Inc()
					i.log.Error(errors.New("failed to cast to Status"), "Received unknown error event", "type", fmt.Sprintf("%T", e.Object))
					continue
				}
				counter.WithLabelValues(i.name, "ERROR", strconv.Itoa(int(status.Code))).Inc()
				if status.Code == int32(410) { // gone
					i.log.Info("Resource version expired, restarting watcher", "kind", status.Kind, "name", status.Details.Name)
					// Schedule reload in a separate goroutine to avoid blocking the watch loop
					go func() {
						if err := i.Reload(); err != nil {
							i.log.Error(err, "Failed to reload after resource version expired")
						}
					}()
					return
				}

				i.log.Error(errors.New(status.Message), "Received error event", "kind", status.Kind, "name", status.Details.Name)
				continue
			case "", watch.Bookmark:
				// Ignore bookmark events
				continue
			}

			obj, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				i.log.Info("Failed to cast object", "type", fmt.Sprintf("%T", e.Object))
				continue
			}
			SanitizeObject(obj)

			timeout := time.After(5 * time.Second)
			select {
			case i.queue <- event{
				typ: string(e.Type),
				obj: obj,
			}:
				queueSize.WithLabelValues(i.name).Inc()
			case <-timeout:
				i.log.Error(nil, "Failed to enqueue event: queue full or blocked",
					"type", e.Type, "name", obj.GetName())
			}

			// Record event processing latency
			eventProcessingLatency.WithLabelValues(i.name).Observe(time.Since(start).Seconds())

		case <-ctx.Done():
			i.log.Info("Watcher stopped")
			watcher.Stop()
			return
		}
	}
}

func (i *NoCacheInformer) resyncLoop(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	for {
		select {
		case <-ticker.C:
			err := i.Reload()
			if err != nil {
				i.log.Error(err, "Failed to resync")
			}
		case <-ctx.Done():
			ticker.Stop()
			return
		}
	}
}

func (i *NoCacheInformer) Start() error {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	if i.resyncPeriod > 0 {
		go i.resyncLoop(i.ctx, i.resyncPeriod)
	}

	// Start worker goroutines for event processing
	for range i.workerCount {
		activeWorkers.WithLabelValues(i.name).Inc()
		go func() {
			i.handlerLoop(i.ctx, i.queue)
			activeWorkers.WithLabelValues(i.name).Dec()
		}()
	}

	go i.startWatcher()

	return nil
}

func (i *NoCacheInformer) stopWatcher() {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	if i.watcher != nil {
		i.watcher.Stop()
		i.watcher = nil
	}
	if i.watcherCancel != nil {
		i.watcherCancel()
		i.watcherCancel = nil
	}
}

func (i *NoCacheInformer) startWatcher() {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	if i.currentlyLoading.Swap(true) {
		return
	}
	defer i.currentlyLoading.Store(false)

	i.resourceVersion = ""

	err := i.list(i.ctx, i.queue)
	if err != nil {
		i.log.Error(err, "Failed to list resources")
	}

	watchCtx, cancel := context.WithCancel(i.ctx)
	i.watcherCancel = cancel

	i.watcher, err = i.k8sClient.Resource(i.gvr).Watch(watchCtx, metav1.ListOptions{
		Watch:               true,
		ResourceVersion:     i.resourceVersion,
		AllowWatchBookmarks: false,
	})

	if err != nil {
		i.log.Error(err, "Failed to start watcher")
		return
	}

	go i.watchLoop(watchCtx, i.watcher)
}

func (i *NoCacheInformer) Ready() bool {
	return i.initDone.Load()
}

func (i *NoCacheInformer) Reload() error {

	if i.currentlyLoading.Load() {
		i.log.Info("Reload already in progress, skipping")
		return nil
	}

	reloads.WithLabelValues(i.name).Inc()
	i.stopWatcher()

	i.startWatcher()

	return nil
}

func (i *NoCacheInformer) Stop() {
	i.stopWatcher()
	i.cancel()
}
