// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"context"
	"fmt"
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

type NoCacheInformer struct {
	ctx             context.Context
	gvr             schema.GroupVersionResource
	k8sClient       dynamic.Interface
	eventHandler    EventHandler
	log             logr.Logger
	bufferSize      int64
	queue           chan event
	initDone        *atomic.Bool
	resourceVersion string
	workerCount     int

	watcher       watch.Interface
	watcherCancel context.CancelFunc
	cancel        context.CancelFunc

	resyncPeriod time.Duration
}

func NewNoCache(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler) *NoCacheInformer {
	log := logr.FromContextOrDiscard(ctx)
	lctx, cancel := context.WithCancel(ctx)
	return &NoCacheInformer{
		ctx:          lctx,
		cancel:       cancel,
		gvr:          gvr,
		k8sClient:    k8sClient,
		eventHandler: eventHandler,
		log:          log.WithName(fmt.Sprintf("Informer:%s/%s", gvr.Group, gvr.Resource)),
		bufferSize:   1000,
		queue:        make(chan event, 1000),
		initDone:     &atomic.Bool{},
		workerCount:  10,
		resyncPeriod: time.Hour,
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
				// TODO: Implement retry logic
				i.log.Error(err, "Failed to handle event", "type", e.typ, "name", e.obj.GetName())
			}
		case <-ctx.Done():
			i.log.Info("Watcher loop stopped")
			return

		}
	}
}

func (i *NoCacheInformer) list(ctx context.Context, inChan chan event) error {
	var continueToken string
	for {
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
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
		i.resourceVersion = list.GetResourceVersion()
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
					i.log.Error(errors.New("failed to cast to Status"), "Received unknown error event", "type", fmt.Sprintf("%T", e.Object))
					continue
				}
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
	i.resourceVersion = ""

	err := i.list(i.ctx, i.queue)
	if err != nil {
		i.log.Error(err, "Failed to list resources")
	}

	i.watcher, err = i.k8sClient.Resource(i.gvr).Watch(i.ctx, metav1.ListOptions{
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
	i.stopWatcher()
	i.initDone.Store(false)
	go i.startWatcher()
	return nil
}

func (i *NoCacheInformer) Stop() {
	i.stopWatcher()
	i.cancel()
}
