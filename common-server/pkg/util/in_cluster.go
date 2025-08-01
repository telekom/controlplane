// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import "os"

const (
	// ServiceAccountTokenFilepath is the default path to the service account token file in Kubernetes
	ServiceAccountTokenFilepath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// IsRunningInCluster checks if the application is running inside a Kubernetes cluster.
// It does this by checking for the existence of the service account token file.
func IsRunningInCluster() bool {
	if _, err := os.Stat(ServiceAccountTokenFilepath); err == nil {
		return true
	}
	return false
}
