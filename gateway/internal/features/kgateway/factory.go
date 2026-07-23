// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var (
	clientCache      = make(map[string]Client)
	clientCacheMutex sync.Mutex
)

// GetClientFor returns the kgateway Client that applies resources for the given
// Gateway. When the Gateway has no RemoteCluster config, it returns the local
// client unchanged. Otherwise it builds (and caches) a client for the target
// cluster from the kubeconfig Secret referenced in the spec.
//
// The kubeconfig Secret is read from the local cluster via local.
var GetClientFor = func(ctx context.Context, local client.Client, gw *gatewayv1.Gateway) (Client, error) {
	rc := gw.Spec.RemoteCluster
	if rc == nil {
		return NewClient(local), nil
	}

	key := cacheKey(rc)
	clientCacheMutex.Lock()
	defer clientCacheMutex.Unlock()
	if c, ok := clientCache[key]; ok {
		return c, nil
	}

	remote, err := newRemoteClient(ctx, local, rc)
	if err != nil {
		return nil, err
	}
	c := NewClient(remote)
	clientCache[key] = c
	return c, nil
}

// cacheKey is a digest of the remote-cluster reference so a changed secret ref
// yields a fresh client.
func cacheKey(rc *gatewayv1.RemoteClusterConfig) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s", rc.SecretRef.Namespace, rc.SecretRef.Name, rc.Key)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func newRemoteClient(ctx context.Context, local client.Client, rc *gatewayv1.RemoteClusterConfig) (client.Client, error) {
	secret := &corev1.Secret{}
	if err := local.Get(ctx, rc.SecretRef.K8s(), secret); err != nil {
		return nil, fmt.Errorf("getting remote-cluster kubeconfig secret %s: %w", rc.SecretRef.String(), err)
	}

	key := rc.Key
	if key == "" {
		key = "kubeconfig"
	}
	kubeconfig, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret %s has no key %q", rc.SecretRef.String(), key)
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("building rest config from kubeconfig: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		return nil, err
	}

	c, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating remote-cluster client: %w", err)
	}
	return c, nil
}
