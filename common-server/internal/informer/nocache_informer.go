// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"context"
	"fmt"
	"hash/fnv"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// [experimental] NoCacheInformer implements the Informer pattern without caching objects.
// It flushes events directly to the event handler, which is useful for resources
// that are too large to store efficiently in memory. It only keeps the
// namespace/name of every known object (O(N) memory) to emit DELETED on relist.
type NoCacheInformer struct {
	ctx             context.Context
	gvr             schema.GroupVersionResource
	name            string
	k8sClient       dynamic.Interface
	eventHandler    EventHandler
	log             logr.Logger
	bufferSize      int64         // Maximum number of items to retrieve per list call
	queues          []chan event  // One event channel per worker; events of one object always use the same channel
	initDone        *atomic.Bool  // Tracks if initial list is complete
	resourceVersion string        // Current resource version for watch operations; guarded by stateMutex
	retryBackoff    wait.Backoff  // Delay between failed watcher (re)starts
	enqueueTimeout  time.Duration // How long a watch event may wait for room in its queue

	watcher       watch.Interface    // The Kubernetes watch client
	watcherCancel context.CancelFunc // Function to cancel the watcher context
	watcherDone   chan struct{}      // Closed when the watch loop of watcher has returned
	cancel        context.CancelFunc // Function to cancel the informer context

	resyncPeriod time.Duration // How often to perform a full resync

	mutex sync.Mutex // Protects access to shared fields and serializes watcher (re)starts

	// knownKeys holds the keys of all objects passed to the event handler and
	// not yet deleted, so a relist can emit DELETED for objects that are gone.
	knownKeys map[types.NamespacedName]struct{}
	// nextDropRelist is the earliest time of the next relist after a dropped event.
	nextDropRelist time.Time
	// dropRelistPending is set while a relist after a dropped event is scheduled.
	dropRelistPending atomic.Bool
	stateMutex        sync.Mutex // Protects knownKeys, resourceVersion and nextDropRelist
}

// Triggers of a watcher (re)start, used in logs.
const (
	triggerStart        = "start"
	triggerWatchClosed  = "watch-closed"
	triggerGone         = "gone"
	triggerResync       = "resync"
	triggerEventDropped = "event-dropped"
)

const defaultEnqueueTimeout = 5 * time.Second

// defaultRetryBackoff controls how often a failed watcher (re)start is retried.
// Retries are unbounded: the delay grows from Duration by Factor until Cap and
// then stays at about Cap plus jitter until the attempt succeeds or the
// informer is stopped. Steps only bounds the growth phase.
var defaultRetryBackoff = wait.Backoff{
	Duration: 1 * time.Second,
	Factor:   2,
	Jitter:   0.2,
	Steps:    10,
	Cap:      30 * time.Second,
}

// NoCacheInformerOptions provides configurable options for the NoCacheInformer
type NoCacheInformerOptions struct {
	BufferSize int64
	// QueueSize is the total event queue capacity, split evenly across workers,
	// at least 1 per worker.
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
		WorkerCount:  runtime.GOMAXPROCS(0), // honors the container CPU limit
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

	workerCount := max(options.WorkerCount, 1)
	queues := make([]chan event, workerCount)
	for w := range queues {
		queues[w] = make(chan event, max(options.QueueSize/workerCount, 1))
	}

	log.Info("Creating new instance")
	return &NoCacheInformer{
		ctx:             ctxWithCancel,
		cancel:          cancel,
		gvr:             gvr,
		k8sClient:       k8sClient,
		eventHandler:    eventHandler,
		log:             log,
		name:            name,
		bufferSize:      options.BufferSize,
		queues:          queues,
		initDone:        &atomic.Bool{},
		resyncPeriod:    options.ResyncPeriod,
		resourceVersion: "0",
		retryBackoff:    defaultRetryBackoff,
		enqueueTimeout:  defaultEnqueueTimeout,
		knownKeys:       make(map[types.NamespacedName]struct{}),
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

// queueFor returns the worker queue of the object with key, so that events of
// one object are handled in order.
func (i *NoCacheInformer) queueFor(key types.NamespacedName) chan event {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key.String()))
	return i.queues[int(h.Sum32())%len(i.queues)]
}

// send enqueues e and blocks until there is room in its queue or ctx is done.
func (i *NoCacheInformer) send(ctx context.Context, e event) error {
	select {
	case i.queueFor(objectKey(e.obj)) <- e:
		queueSize.WithLabelValues(i.name).Inc()
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "failed to enqueue event")
	}
}

func (i *NoCacheInformer) list(ctx context.Context) error {
	var continueToken, resourceVersion string

	timeout := i.resyncPeriod * 9 / 10
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	i.log.Info("Listing resources", "timeout", timeout.String())
	start := time.Now()
	listed := make(map[types.NamespacedName]struct{})

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
				key := objectKey(&item)
				listed[key] = struct{}{}
				if err := i.send(ctx, event{typ: "ADDED", obj: &item}); err != nil {
					return err
				}
				i.trackKey(watch.Added, key)
			}

			// Give the system a small breather between batches
			if end < len(list.Items) && len(list.Items) > batchSize {
				time.Sleep(time.Millisecond * 50)
			}
		}

		resourceVersion = list.GetResourceVersion()
		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}

	for _, key := range i.unlistedKeys(listed) {
		obj := &unstructured.Unstructured{}
		obj.SetNamespace(key.Namespace)
		obj.SetName(key.Name)
		if err := i.send(ctx, event{typ: "DELETED", obj: obj}); err != nil {
			return err
		}
		i.trackKey(watch.Deleted, key)
	}

	// Only a complete list yields a resource version to watch from, so a
	// failed list is retried as a full list.
	i.setResourceVersion(resourceVersion)
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
				if ctx.Err() != nil {
					// Closed by stopWatcher or Stop; nothing to reload.
					return
				}
				i.log.Info("Watcher channel closed, restarting watcher")
				// Schedule reload in a separate goroutine to avoid blocking the watch loop
				go i.retry(triggerWatchClosed, i.Reload)
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
					i.log.Info("Resource version expired, restarting watcher", "resourceVersion", i.getResourceVersion())
					// Reset resource version to force a full relist
					i.setResourceVersion("0")
					// Schedule reload in a separate goroutine to avoid blocking the watch loop
					go i.retry(triggerGone, i.Reload)
					return
				}

				name := ""
				if status.Details != nil {
					name = status.Details.Name
				}
				i.log.Error(errors.New(status.Message), "Received error event", "reason", status.Reason, "code", status.Code, "name", name)
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
			SanitizeObject(obj)

			select {
			case i.queueFor(objectKey(obj)) <- event{
				typ: string(e.Type),
				obj: obj,
			}:
				queueSize.WithLabelValues(i.name).Inc()
				i.trackKey(e.Type, objectKey(obj))
				// Advance only for queued events, so a restarted watch resends the others.
				i.setResourceVersion(obj.GetResourceVersion())
			case <-time.After(i.enqueueTimeout):
				// A full relist repairs the state the dropped event would have changed.
				i.setResourceVersion("0")
				watcher.Stop()
				if !i.dropRelistPending.CompareAndSwap(false, true) {
					i.log.Error(nil, "Failed to enqueue event: queue full or blocked, relist already pending",
						"type", e.Type, "name", obj.GetName())
					return
				}
				delay := i.dropRelistDelay()
				i.log.Error(nil, "Failed to enqueue event: queue full or blocked, relisting",
					"type", e.Type, "name", obj.GetName(), "delay", delay.String())
				go func() {
					select {
					case <-time.After(delay):
						i.dropRelistPending.Store(false)
						i.retry(triggerEventDropped, i.Reload)
					case <-i.ctx.Done():
					}
				}()
				return
			case <-ctx.Done():
				return
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

			i.retry(triggerResync, i.Reload)
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
	for _, queue := range i.queues {
		activeWorkers.WithLabelValues(i.name).Inc()
		go func() {
			i.handlerLoop(i.ctx, queue)
			activeWorkers.WithLabelValues(i.name).Dec()
		}()
	}

	go i.retry(triggerStart, i.startWatcher)

	return nil
}

// retry runs fn with capped, jittered exponential backoff until it succeeds
// or the informer is stopped. trigger names the cause in logs.
func (i *NoCacheInformer) retry(trigger string, fn func() error) {
	err := i.retryBackoff.DelayFunc().Until(i.ctx, true, true, func(context.Context) (bool, error) {
		if err := fn(); err != nil {
			i.log.Error(err, "Failed to (re)start watcher, retrying", "trigger", trigger)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		i.log.V(1).Info("Stopped retrying watcher", "trigger", trigger, "reason", err.Error())
	}
}

func (i *NoCacheInformer) stopWatcher() {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	i.stopWatcherLocked()
}

// stopWatcherLocked requires i.mutex.
func (i *NoCacheInformer) stopWatcherLocked() {
	if i.watcherCancel != nil {
		i.watcherCancel()
		i.watcherCancel = nil
	}
	if i.watcher != nil {
		i.watcher.Stop()
		i.watcher = nil
	}
	if i.watcherDone != nil {
		// The old watch loop must not enqueue events once a new list has started.
		<-i.watcherDone
		i.watcherDone = nil
	}
}

// startWatcher replaces any running watcher. i.mutex serializes concurrent
// (re)starts, so none of them is skipped.
func (i *NoCacheInformer) startWatcher() error {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	i.stopWatcherLocked()

	// Perform an initial list if we don't have a valid resource version
	// (i.e. at startup or after a forced reload)
	if len(i.getResourceVersion()) < 2 {
		err := i.list(i.ctx)
		if err != nil {
			return errors.Wrap(err, "failed to start watcher")
		}
	}

	watchCtx, cancel := context.WithCancel(i.ctx)
	watcher, err := i.k8sClient.Resource(i.gvr).Watch(watchCtx, metav1.ListOptions{
		Watch:               true,
		ResourceVersion:     i.getResourceVersion(),
		AllowWatchBookmarks: true,
	})
	if err != nil {
		cancel()
		if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
			i.setResourceVersion("0")
		}
		return errors.Wrap(err, "failed to start watcher")
	}

	done := make(chan struct{})
	i.watcher, i.watcherCancel, i.watcherDone = watcher, cancel, done
	go func() {
		defer close(done)
		i.watchLoop(watchCtx, watcher)
	}()

	return nil
}

func (i *NoCacheInformer) Ready() bool {
	return i.initDone.Load()
}

func (i *NoCacheInformer) Reload() error {
	i.log.Info("Reloading informer")

	reloads.WithLabelValues(i.name).Inc()

	err := i.startWatcher()
	if err != nil {
		return errors.Wrap(err, "failed to restart watcher")
	}

	return nil
}

func (i *NoCacheInformer) Stop() {
	// Cancel first so a running list or retry returns and releases i.mutex.
	i.cancel()
	i.stopWatcher()
}

// dropRelistDelay returns how long to wait before the relist after a dropped
// event, so that sustained overload relists at most once per retryBackoff.Cap.
// No watch runs until that relist, so updates of all objects are delayed by up
// to retryBackoff.Cap.
func (i *NoCacheInformer) dropRelistDelay() time.Duration {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	now := time.Now()
	next := i.nextDropRelist
	if next.Before(now) {
		next = now
	}
	i.nextDropRelist = next.Add(i.retryBackoff.Cap)
	return next.Sub(now)
}

func (i *NoCacheInformer) getResourceVersion() string {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	return i.resourceVersion
}

func (i *NoCacheInformer) setResourceVersion(rv string) {
	if rv == "" {
		return
	}
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.resourceVersion = rv
}

func objectKey(obj *unstructured.Unstructured) types.NamespacedName {
	return types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
}

// trackKey records an object passed to the event handler.
func (i *NoCacheInformer) trackKey(typ watch.EventType, key types.NamespacedName) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if typ == watch.Deleted {
		delete(i.knownKeys, key)
		return
	}
	i.knownKeys[key] = struct{}{}
}

// unlistedKeys returns all known keys missing from listed.
func (i *NoCacheInformer) unlistedKeys(listed map[types.NamespacedName]struct{}) []types.NamespacedName {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	var gone []types.NamespacedName
	for key := range i.knownKeys {
		if _, ok := listed[key]; !ok {
			gone = append(gone, key)
		}
	}
	return gone
}
