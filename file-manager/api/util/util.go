// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"os"
	"time"
)

var (
	ClientName = "file-manager"
	// todo check the api names
	ReplacePattern = `^\/api\/v1\/(upload|download)\/(?P<redacted>.*)$`

	DefaultClientTimeout = 5 * time.Second
	ClientTimeout        = os.Getenv("CLIENT_TIMEOUT")
)

// todo - might be shared functionality - move to common ?
// IsRunningInCluster checks if the application is running in a Kubernetes cluster
func IsRunningInCluster() bool {
	_, ok := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	return ok
}
