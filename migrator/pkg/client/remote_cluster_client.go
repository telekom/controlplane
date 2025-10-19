// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// RemoteClusterConfig holds configuration for connecting to remote cluster
type RemoteClusterConfig struct {
	APIServer  string
	Token      string
	CAData     []byte
	Kubeconfig []byte // Optional: full kubeconfig
}

// NewRemoteClusterClient creates a new client for accessing the remote cluster
// This function loads configuration from a Kubernetes secret
func NewRemoteClusterClient(
	reader ctrlclient.Reader,
	secretName string,
	secretNamespace string,
	scheme *runtime.Scheme,
) (ctrlclient.Client, error) {
	// Fetch the secret containing remote cluster credentials
	secret := &corev1.Secret{}
	err := reader.Get(context.Background(), types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}, secret)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get remote cluster secret")
	}

	// Extract configuration from secret
	server := string(secret.Data["server"])
	token := string(secret.Data["token"])
	caData := secret.Data["ca.crt"]

	if server == "" {
		return nil, errors.New("server not found in secret")
	}
	if token == "" {
		return nil, errors.New("token not found in secret")
	}

	// Create remote client configuration
	config := &RemoteClusterConfig{
		APIServer: server,
		Token:     token,
		CAData:    caData,
	}

	// Check if kubeconfig is provided
	if kubeconfig, ok := secret.Data["kubeconfig"]; ok {
		config.Kubeconfig = kubeconfig
	}

	// Create and return the remote client
	return NewRemoteClusterClientFromConfig(config, scheme)
}

// NewRemoteClusterClientFromConfig creates a client from explicit configuration
func NewRemoteClusterClientFromConfig(config *RemoteClusterConfig, scheme *runtime.Scheme) (ctrlclient.Client, error) {
	var restConfig *rest.Config
	var err error

	// Try to use kubeconfig if provided
	if len(config.Kubeconfig) > 0 {
		restConfig, err = clientcmd.RESTConfigFromKubeConfig(config.Kubeconfig)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create config from kubeconfig")
		}
	} else {
		// Build config from individual components
		restConfig = &rest.Config{
			Host:        config.APIServer,
			BearerToken: config.Token,
		}

		if len(config.CAData) > 0 {
			restConfig.TLSClientConfig = rest.TLSClientConfig{
				CAData: config.CAData,
			}
		}
	}

	// Create Kubernetes client with provided scheme
	client, err := ctrlclient.New(restConfig, ctrlclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kubernetes client")
	}

	return client, nil
}
