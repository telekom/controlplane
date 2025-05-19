// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"context"
	"fmt"
	"reflect"
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

type EventHandler interface {
	OnCreate(ctx context.Context, obj *unstructured.Unstructured) error
	OnUpdate(ctx context.Context, obj *unstructured.Unstructured) error
	OnDelete(ctx context.Context, obj *unstructured.Unstructured) error
}

type Informer struct {
	ctx            context.Context
	gvr            schema.GroupVersionResource
	k8sClient      dynamic.Interface
	eventHandler   EventHandler
	log            logr.Logger
	reloadInterval time.Duration
	informer       cache.SharedIndexInformer
}

func New(ctx context.Context, gvr schema.GroupVersionResource, k8sClient dynamic.Interface, eventHandler EventHandler) *Informer {
	log := logr.FromContextOrDiscard(ctx)
	return &Informer{
		ctx:            ctx,
		gvr:            gvr,
		k8sClient:      k8sClient,
		eventHandler:   eventHandler,
		log:            log.WithName(fmt.Sprintf("Informer:%s/%s", gvr.Group, gvr.Resource)),
		reloadInterval: 600 * time.Second,
	}
}

func (i *Informer) Start() error {
	listOpts := func(lo *metav1.ListOptions) {}
	indexers := cache.Indexers{}
	namespace := ""

	i.informer = dynamicinformer.NewFilteredDynamicInformer(i.k8sClient, i.gvr, namespace, i.reloadInterval, indexers, listOpts).Informer()
	_, err := i.informer.AddEventHandlerWithResyncPeriod(wrapEventHandler(i.ctx, i.log, i.eventHandler), i.reloadInterval)
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
		i.log.Error(err, "watch error")
	})
	if err != nil {
		return errors.Wrapf(err, "failed to set watch error handler for %s", i.gvr)
	}

	go i.informer.Run(i.ctx.Done())
	return nil
}

func (i *Informer) Ready() bool {
	return i.informer.HasSynced()
}

func SanitizeObject(obj *unstructured.Unstructured) {
	metadata, ok := obj.Object["metadata"].(map[string]any)
	if !ok {
		panic(errors.New("failed to cast metadata"))
	}

	delete(metadata, "managedFields")
}

func wrapEventHandler(ctx context.Context, log logr.Logger, eh EventHandler) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			o, ok := obj.(*unstructured.Unstructured)
			if !ok {
				log.Error(fmt.Errorf("invalid type %s", reflect.TypeOf(obj)), "failed to cast object")
				return
			}
			if err := eh.OnCreate(ctx, o); err != nil {
				log.Error(err, "failed to handle create event")
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
			}
		},
	}
}
