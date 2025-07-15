// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics

type CollectionInterface interface {
	RecordCacheHit()
	RecordCacheMiss(reason string)
}

// Global metrics collection instance
var Collection CollectionInterface
