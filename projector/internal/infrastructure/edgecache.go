// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"fmt"

	"github.com/dgraph-io/ristretto/v2"
)

// EdgeCache is a Ristretto-based cache for FK lookups. It maps
// (entityType, lookupKey) -> database primary key (int), avoiding repeated
// DB queries when resolving edges during upsert operations.
type EdgeCache struct {
	cache *ristretto.Cache[string, int]
}

// NewEdgeCache creates an EdgeCache with the given Ristretto configuration.
// Parameters:
//   - numCounters: number of counters for admission policy (~10x expected items)
//   - maxCost: maximum cache memory budget in bytes
//   - bufferItems: internal per-Get buffer size
func NewEdgeCache(numCounters, maxCost, bufferItems int64) (*EdgeCache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, int]{
		NumCounters: numCounters,
		MaxCost:     maxCost,
		BufferItems: bufferItems,
	})
	if err != nil {
		return nil, fmt.Errorf("creating edge cache: %w", err)
	}
	return &EdgeCache{cache: cache}, nil
}

// cacheKey builds a composite key from entity type and lookup key.
func cacheKey(entityType, lookupKey string) string {
	return entityType + ":" + lookupKey
}

// Get retrieves the cached primary key for the given entity type and lookup key.
// Returns the primary key and true on hit, or 0 and false on miss.
func (c *EdgeCache) Get(entityType, lookupKey string) (int, bool) {
	val, found := c.cache.Get(cacheKey(entityType, lookupKey))
	return val, found
}

// Set stores the primary key for the given entity type and lookup key.
// Cost is set to 1 (each entry counts equally toward the budget).
func (c *EdgeCache) Set(entityType, lookupKey string, pk int) {
	c.cache.Set(cacheKey(entityType, lookupKey), pk, 1)
}

// Del removes the cached entry for the given entity type and lookup key.
func (c *EdgeCache) Del(entityType, lookupKey string) {
	c.cache.Del(cacheKey(entityType, lookupKey))
}

// Close stops the cache's internal goroutines and releases resources.
func (c *EdgeCache) Close() {
	c.cache.Close()
}

// Wait waits for all pending Set operations to be applied.
// This is primarily useful in tests to ensure deterministic behavior.
func (c *EdgeCache) Wait() {
	c.cache.Wait()
}
