// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// SetTestTimings overrides the retry backoff and the enqueue timeout. It must
// be called before Start.
func (i *NoCacheInformer) SetTestTimings(backoff wait.Backoff, enqueueTimeout time.Duration) {
	i.retryBackoff = backoff
	i.enqueueTimeout = enqueueTimeout
}
