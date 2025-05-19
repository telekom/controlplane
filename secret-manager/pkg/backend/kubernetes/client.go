// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	cache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
)

func NewCachedClient(ctx context.Context, restCfg *rest.Config) (client.Client, error) {
	restCfg.UserAgent = "secret-manager"

	scheme := runtime.NewScheme()
	err := (&crscheme.Builder{
		GroupVersion: corev1.SchemeGroupVersion,
	}).Register(&corev1.Secret{}, &corev1.SecretList{}).AddToScheme(scheme)

	if err != nil {
		return nil, errors.Wrap(err, "failed to register scheme")
	}

	managedOnly, err := labels.NewRequirement("app.kubernetes.io/managed-by", selection.DoubleEquals, []string{"secret-manager"})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create label requirement")
	}

	k8sCache, err := cache.New(restCfg, cache.Options{
		Scheme:               scheme,
		DefaultLabelSelector: labels.NewSelector().Add(*managedOnly),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cache")
	}

	go func() {
		err = k8sCache.Start(ctx)
	}()

	if err != nil {
		return nil, errors.Wrap(err, "failed to start cache")
	}

	if !k8sCache.WaitForCacheSync(ctx) {
		return nil, errors.New("failed to sync cache")
	}

	k8sClient, err := client.New(restCfg, client.Options{
		Cache: &client.CacheOptions{
			DisableFor: []client.Object{},
			Reader:     k8sCache,
		},
		Scheme: scheme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Kubernetes client")
	}

	return k8sClient, err
}
