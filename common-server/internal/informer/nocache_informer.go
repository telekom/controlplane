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
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// [experimental] NoCacheInformer implements the Informer pattern without keeping a local cache.
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
	BufferSize           int64
	QueueSize            int
	WorkerCount          int
	ResyncPeriod         time.Duration
	PrometheusRegisterer prometheus.Registerer
}

// DefaultNoCacheInformerOptions returns the default options for a NoCacheInformer
func DefaultNoCacheInformerOptions() NoCacheInformerOptions {
	return NoCacheInformerOptions{
		BufferSize:   1000,
		QueueSize:    200,
		WorkerCount:  runtime.NumCPU(),
		ResyncPeriod: 1 * time.Hour,
	}
}

// [experimental] NewNoCache creates a new informer that does not use a local cache.
func NewNoCache(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler) *NoCacheInformer {
	return NewNoCacheWithOptions(ctx, gvr, k8sClient, eventHandler, DefaultNoCacheInformerOptions())
}

// [experimental] NewNoCacheWithOptions creates a new informer that does not use a local cache with custom options.
func NewNoCacheWithOptions(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler, options NoCacheInformerOptions) *NoCacheInformer {
	name := fmt.Sprintf("NoCacheInformer:%s/%s", strings.ToLower(gvr.Group), strings.ToLower(gvr.Resource))
	log := logr.FromContextOrDiscard(ctx).WithName(name)
	ctxWithCancel, cancel := context.WithCancel(ctx)

	if options.PrometheusRegisterer != nil {
		Register(options.PrometheusRegisterer)
	} else {
		Register(prometheus.DefaultRegisterer)
	}

	log.Info("Creating new instance")
	return &NoCacheInformer{
		ctx:              ctxWithCancel,
		cancel:           cancel,
		gvr:              gvr,
		k8sClient:        k8sClient,
		eventHandler:     eventHandler,
		log:              log,
		name:             name,
		bufferSize:       options.BufferSize,
		queue:            make(chan event, options.QueueSize),
		initDone:         &atomic.Bool{},
		currentlyLoading: &atomic.Bool{},
		workerCount:      options.WorkerCount,
		resyncPeriod:     options.ResyncPeriod,
		resourceVersion:  "0",
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
			i.log.V(2).Info("Handler loop stopped")
			return
		}
	}
}

func (i *NoCacheInformer) list(ctx context.Context, inChan chan event) error {
	var continueToken string

	timeout := i.resyncPeriod * 9 / 10
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	i.log.Info("Listing resources", "timeout", timeout.String())
	start := time.Now()

	for {
		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "failed to list resources")
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

		i.setResourceVersion(list.GetResourceVersion())
		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}
	i.initDone.Store(true)
	listOperationDuration.WithLabelValues(i.name).Set(time.Since(start).Seconds())

	return nil
}

func (i *NoCacheInformer) watchLoop(ctx context.Context, watcher watch.Interface) {
	for {
		select {
		case e, ok := <-watcher.ResultChan():
			watchLoopIterations.WithLabelValues(i.name).Inc()
			if !ok {
				i.log.Info("Watcher channel closed, restarting watcher")
				// Schedule reload in a separate goroutine to avoid blocking the watch loop
				go func() {
					if err := i.Reload(); err != nil {
						i.log.Error(err, "Failed to reload after watcher channel closed")
					}
				}()
				return
			}
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
					i.log.Info("Resource version expired, restarting watcher", "resourceVersion", i.resourceVersion)
					// Reset resource version to force a full relist
					i.setResourceVersion("0")
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
			case watch.Bookmark:
				obj, ok := e.Object.(metav1.Object)
				if !ok {
					i.log.V(1).Info("Failed to cast bookmark object", "type", fmt.Sprintf("%T", e.Object))
					continue
				}

				i.setResourceVersion(obj.GetResourceVersion())
				i.log.V(1).Info("Received bookmark", "resourceVersion", obj.GetResourceVersion())
				counter.WithLabelValues(i.name, "BOOKMARK", "0").Inc()
				continue

			case "":
				// Ignore empty events
				continue
			}

			obj, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				i.log.Info("Failed to cast object", "type", fmt.Sprintf("%T", e.Object))
				continue
			}
			i.setResourceVersion(obj.GetResourceVersion())

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
			i.log.V(1).Info("Watcher stopped")
			watcher.Stop()
			return
		}
	}
}

func (i *NoCacheInformer) resyncLoop(ctx context.Context, period time.Duration) {
	jitteredPeriod := wait.Jitter(period, 0.2)
	ticker := time.NewTicker(jitteredPeriod)
	defer ticker.Stop()
	i.log.Info("Starting resync loop", "period", jitteredPeriod.String())
	for {
		select {
		case <-ticker.C:
			newJitteredPeriod := wait.Jitter(period, 0.2)
			ticker.Reset(newJitteredPeriod)
			i.log.Info("Resyncing informer", "nextPeriod", newJitteredPeriod.String())

			err := i.Reload()
			if err != nil {
				i.log.Error(err, "Failed to resync")
			}
		case <-ctx.Done():
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

	go func() {
		err := i.startWatcher()
		if err != nil {
			i.log.Error(err, "Failed to start watcher")
		}
	}()

	return nil
}

func (i *NoCacheInformer) stopWatcher() {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	if i.watcherCancel != nil {
		i.watcherCancel()
		i.watcherCancel = nil
	}
	if i.watcher != nil {
		i.watcher.Stop()
		i.watcher = nil
	}
}

func (i *NoCacheInformer) startWatcher() (err error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	if i.currentlyLoading.Swap(true) {
		// Another load is already in progress
		return nil
	}
	defer i.currentlyLoading.Store(false)

	// Perform an initial list if we don't have a valid resource version
	// (i.e. at startup or after a forced reload)
	if len(i.resourceVersion) < 2 {
		err := i.list(i.ctx, i.queue)
		if err != nil {
			return errors.Wrap(err, "failed to start watcher")
		}
	}

	watchCtx, cancel := context.WithCancel(i.ctx)
	i.watcherCancel = cancel

	// TODO: this should be wrapped in a backoff loop to handle transient errors
	// see wait.Backoff

	i.watcher, err = i.k8sClient.Resource(i.gvr).Watch(watchCtx, metav1.ListOptions{
		Watch:               true,
		ResourceVersion:     i.resourceVersion,
		AllowWatchBookmarks: true,
	})

	if err != nil {
		i.setResourceVersion("0")
		return errors.Wrap(err, "failed to start watcher")
	}

	go i.watchLoop(watchCtx, i.watcher)

	return nil
}

func (i *NoCacheInformer) Ready() bool {
	return i.initDone.Load()
}

func (i *NoCacheInformer) Reload() error {
	if i.currentlyLoading.Load() {
		i.log.Info("Reload already in progress, skipping")
		return nil
	}
	i.log.Info("Reloading informer")

	reloads.WithLabelValues(i.name).Inc()
	i.stopWatcher()

	err := i.startWatcher()
	if err != nil {
		return errors.Wrap(err, "failed to restart watcher")
	}

	return nil
}

func (i *NoCacheInformer) Stop() {
	i.stopWatcher()
	i.cancel()
}

func (i *NoCacheInformer) setResourceVersion(rv string) {
	if rv == "" {
		return
	}
	i.resourceVersion = rv
}
