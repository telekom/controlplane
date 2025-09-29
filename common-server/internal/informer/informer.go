// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/dynamic/dynamicinformer"
)

type Informer interface {
	Start() error
	Ready() bool
}

type EventHandler interface {
	OnCreate(ctx context.Context, obj *unstructured.Unstructured) error
	OnUpdate(ctx context.Context, obj *unstructured.Unstructured) error
	OnDelete(ctx context.Context, obj *unstructured.Unstructured) error
}

type KubeInformer struct {
	ctx            context.Context
	gvr            schema.GroupVersionResource
	k8sClient      dynamic.Interface
	eventHandler   EventHandler
	log            logr.Logger
	reloadInterval time.Duration
	informer       cache.SharedIndexInformer
	name           string
}

func New(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler) *KubeInformer {
	log := logr.FromContextOrDiscard(ctx)
	name := fmt.Sprintf("Informer:%s/%s", strings.ToLower(gvr.Group), strings.ToLower(gvr.Resource))
	return &KubeInformer{
		ctx:            ctx,
		gvr:            gvr,
		k8sClient:      k8sClient,
		eventHandler:   eventHandler,
		name:           name,
		log:            log.WithName(name),
		reloadInterval: 600 * time.Second,
	}
}

func (i *KubeInformer) Start() error {
	listOpts := func(lo *metav1.ListOptions) {}
	indexers := cache.Indexers{}
	namespace := ""

	i.informer = dynamicinformer.NewFilteredDynamicInformer(i.k8sClient, i.gvr, namespace, i.reloadInterval, indexers, listOpts).Informer()
	_, err := i.informer.AddEventHandlerWithResyncPeriod(i.wrapEventHandler(i.ctx, i.log, i.eventHandler), i.reloadInterval)
	if err != nil {
		return errors.Wrapf(err, "failed to add event handler for %s", i.gvr)
	}

	err = i.informer.SetTransform(func(i any) (any, error) {
		o, ok := i.(*unstructured.Unstructured)
		if !ok {
			return nil, errors.New("failed to cast object")
		}

		SanitizeObject(o)
		return o, nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed to set transform for %s", i.gvr)
	}

	err = i.informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		counter.WithLabelValues(i.name, "ERROR", "1").Inc()
		i.log.Error(err, "watch error")
	})
	if err != nil {
		return errors.Wrapf(err, "failed to set watch error handler for %s", i.gvr)
	}

	go i.informer.Run(i.ctx.Done())
	return nil
}

func (i *KubeInformer) Ready() bool {
	return i.informer.HasSynced()
}

func SanitizeObject(obj *unstructured.Unstructured) {
	metadata, ok := obj.Object["metadata"].(map[string]any)
	if !ok {
		panic(errors.New("failed to cast metadata"))
	}

	delete(metadata, "managedFields")
	annotations, ok := metadata["annotations"].(map[string]any)
	if ok {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	}
}

func (i *KubeInformer) wrapEventHandler(ctx context.Context, log logr.Logger, eh EventHandler) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			o, ok := obj.(*unstructured.Unstructured)
			if !ok {
				log.Error(fmt.Errorf("invalid type %s", reflect.TypeOf(obj)), "failed to cast object")
				return
			}
			if err := eh.OnCreate(ctx, o); err != nil {
				log.Error(err, "failed to handle create event")
				counter.WithLabelValues(i.name, "ADDED", "1").Inc()
			} else {
				counter.WithLabelValues(i.name, "ADDED", "0").Inc()
			}

		},
		UpdateFunc: func(oldObj, newObj any) {
			o, ok := newObj.(*unstructured.Unstructured)
			if !ok {
				log.Error(fmt.Errorf("invalid type %s", reflect.TypeOf(newObj)), "failed to cast object")
				return
			}
			if err := eh.OnUpdate(ctx, o); err != nil {
				log.Error(err, "failed to handle update event")
				counter.WithLabelValues(i.name, "MODIFIED", "1").Inc()
			} else {
				counter.WithLabelValues(i.name, "MODIFIED", "0").Inc()
			}

		},
		DeleteFunc: func(obj any) {
			o, ok := obj.(*unstructured.Unstructured)
			if !ok {
				log.Error(fmt.Errorf("invalid type %s", reflect.TypeOf(obj)), "failed to cast object")
				return
			}
			if err := eh.OnDelete(ctx, o); err != nil {
				log.Error(err, "failed to handle delete event")
				counter.WithLabelValues(i.name, "DELETED", "1").Inc()
			} else {
				counter.WithLabelValues(i.name, "DELETED", "0").Inc()
			}
		},
	}
}
