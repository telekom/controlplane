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

// [experimental] NoCacheInformer implement the Informer pattern but it does not keep a local cache.
// It will flush the events directly to the event handler.
// This is useful for resources that are really large and would consume too much memory in a local cache.
type NoCacheInformer struct {
	ctx                context.Context
	gvr                schema.GroupVersionResource
	name               string
	k8sClient          dynamic.Interface
	eventHandler       EventHandler
	log                logr.Logger
	bufferSize         int64
	queue              chan event
	initDone           *atomic.Bool
	currentlyReloading *atomic.Bool
	resourceVersion    string
	workerCount        int

	watcher       watch.Interface
	watcherCancel context.CancelFunc
	cancel        context.CancelFunc

	resyncPeriod time.Duration

	mutex sync.Mutex
}

// [experimental] NewNoCache creates a new informer that does not use a local cache.
func NewNoCache(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler) *NoCacheInformer {
	log := logr.FromContextOrDiscard(ctx)
	lctx, cancel := context.WithCancel(ctx)
	name := fmt.Sprintf("NoCacheInformer:%s/%s", strings.ToLower(gvr.Group), strings.ToLower(gvr.Resource))
	return &NoCacheInformer{
		ctx:                lctx,
		cancel:             cancel,
		gvr:                gvr,
		k8sClient:          k8sClient,
		eventHandler:       eventHandler,
		log:                log.WithName(name),
		name:               name,
		bufferSize:         1000,
		queue:              make(chan event, 200),
		initDone:           &atomic.Bool{},
		currentlyReloading: &atomic.Bool{},
		workerCount:        runtime.NumCPU(),
		resyncPeriod:       time.Hour,
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

			queueSize.WithLabelValues(i.name).Dec()

		case <-ctx.Done():
			i.log.Info("Watcher loop stopped")
			return

		}
	}
}

func (i *NoCacheInformer) list(ctx context.Context, inChan chan event) error {
	var continueToken string

	if i.currentlyReloading.Swap(true) {
		// another reload is already in progress
		return nil
	}
	defer i.currentlyReloading.Store(false)

	ctx, cancel := context.WithTimeout(ctx, i.resyncPeriod*9/10)
	defer cancel()

	i.log.Info("Listing resources")
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

		for _, item := range list.Items {
			inChan <- event{
				typ: "ADDED",
				obj: item.DeepCopy(),
			}
			queueSize.WithLabelValues(i.name).Inc()
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
					// TODO: add a metrics for this
					i.log.Info("Resource version expired, restarting watcher", "kind", status.Kind, "name", status.Details.Name)
					if err := i.Reload(); err != nil {
						i.log.Error(err, "Failed to reload after resource version expired")
					}
					return
				}

				i.log.Error(errors.New(status.Message), "Received error event", "kind", status.Kind, "name", status.Details.Name)

				continue
			case "":
				// Ignore empty events
				continue
			case watch.Bookmark:
				// Ignore bookmark events
				continue
			}

			obj, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				i.log.Info("Failed to cast object", "type", fmt.Sprintf("%T", e.Object))
				continue
			}
			SanitizeObject(obj)
			i.queue <- event{
				typ: string(e.Type),
				obj: obj.DeepCopy(),
			}
			queueSize.WithLabelValues(i.name).Inc()

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

	for range i.workerCount {
		go i.handlerLoop(i.ctx, i.queue)
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

	i.resourceVersion = ""

	err := i.list(i.ctx, i.queue)
	if err != nil {
		i.log.Error(err, "Failed to list resources")
	}

	wctx, cancel := context.WithCancel(i.ctx)
	i.watcherCancel = cancel

	i.watcher, err = i.k8sClient.Resource(i.gvr).Watch(wctx, metav1.ListOptions{
		Watch:           true,
		ResourceVersion: i.resourceVersion,
	})

	if err != nil {
		i.log.Error(err, "Failed to start watcher")
		return
	}

	for range i.workerCount {
		go i.watchLoop(i.ctx, i.watcher)
	}

}

func (i *NoCacheInformer) Ready() bool {
	return i.initDone.Load()
}

func (i *NoCacheInformer) Reload() error {
	reloads.WithLabelValues(i.name).Inc()
	i.stopWatcher()
	go i.startWatcher()
	return nil
}

func (i *NoCacheInformer) Stop() {
	i.stopWatcher()
	i.cancel()
}
