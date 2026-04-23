// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/go-logr/logr"
	"golang.org/x/sync/singleflight"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache/metrics"
)

var _ backend.Backend[backend.SecretId, backend.Secret[backend.SecretId]] = (*CachedBackend[backend.SecretId, backend.Secret[backend.SecretId]])(nil)

type CachedBackend[T backend.SecretId, S backend.Secret[T]] struct {
	Backend backend.Backend[T, S]
	Cache   *ristretto.Cache[string, S]
	ttl     time.Duration
	group   singleflight.Group
}

type CacheOptions struct {
	TTL           time.Duration
	MaxCost       int64
	ExpectedItems int64
}

type CacheOption func(*CacheOptions)

// WithTTL sets the time-to-live for cache entries.
// After the TTL expires, the cache entry will be evicted and subsequent reads will fetch fresh data from the backend.
func WithTTL(ttl time.Duration) CacheOption {
	return func(opts *CacheOptions) {
		opts.TTL = ttl
	}
}

// WithMaxCost sets the maximum cost of the cache in bytes.
// The cost of each item is calculated as the size of the secret value plus the size of the cache key.
func WithMaxCost(maxCost int64) CacheOption {
	return func(opts *CacheOptions) {
		opts.MaxCost = maxCost
	}
}

// WithExpectedItems sets the expected number of items in the cache, which is used to calculate the number of counters for the Ristretto cache.
func WithExpectedItems(expectedItems int64) CacheOption {
	return func(opts *CacheOptions) {
		opts.ExpectedItems = expectedItems
	}
}

func NewCachedBackend[T backend.SecretId, S backend.Secret[T]](backend backend.Backend[T, S], opts ...CacheOption) *CachedBackend[T, S] {
	options := &CacheOptions{
		TTL:           2 * time.Hour, // default TTL of 2 hours
		MaxCost:       100 << 20,     // 100MB
		ExpectedItems: 50_000,        // expect around 50k secrets, adjust as needed based on typical secret size and total cache size
	}
	for _, opt := range opts {
		opt(options)
	}

	// Calculate NumCounters as 10x the expected number of items to allow for good hit rates without excessive memory usage.
	numCounters := options.ExpectedItems * 10

	cache, err := ristretto.NewCache(&ristretto.Config[string, S]{
		NumCounters: numCounters,     // number of keys to track frequency of.
		MaxCost:     options.MaxCost, // maximum cost of cache (in bytes).
		BufferItems: 64,              // number of keys per Get buffer.
	})
	if err != nil {
		panic(err)
	}

	return &CachedBackend[T, S]{
		Backend: backend,
		Cache:   cache,
		ttl:     options.TTL,
	}
}

// CacheSizeBytes returns the current cache usage in bytes (MaxCost - RemainingCost).
// This is intended to be passed as a metrics.CacheSizeFunc for Prometheus gauge registration.
func (c *CachedBackend[T, S]) CacheSizeBytes() float64 {
	return float64(c.Cache.MaxCost() - c.Cache.RemainingCost())
}

// invalidateParent removes the parent secret's cache and singleflight entries
// when a sub-secret is modified. This prevents stale reads of the parent document
// after a sub-secret write changes the underlying stored value.
func (c *CachedBackend[T, S]) invalidateParent(id T) {
	if id.SubPath() != backend.NoSubPath {
		parentKey := id.ParentId().CacheKey()
		c.group.Forget(parentKey)
		c.Cache.Del(parentKey)
	}
}

// Delete implements backend.Backend.
func (c *CachedBackend[T, S]) Delete(ctx context.Context, id T) error {
	cacheKey := id.CacheKey()
	c.group.Forget(cacheKey)
	c.Cache.Del(cacheKey)
	c.invalidateParent(id)
	return c.Backend.Delete(ctx, id)
}

// Get implements backend.Backend.
func (c *CachedBackend[T, S]) Get(ctx context.Context, id T) (S, error) {
	log := logr.FromContextOrDiscard(ctx)
	cacheKey := id.CacheKey()

	cachedItem, ok := c.Cache.Get(cacheKey)
	if ok {
		if len(cachedItem.Value()) > 0 {
			metrics.RecordCacheHit("get", "success")
			return cachedItem.Copy().(S), nil
		}
		metrics.RecordCacheMiss("get", "empty_value")
	} else {
		metrics.RecordCacheMiss("get", "not_found")
	}

	// Deduplicate concurrent backend reads for the same key.
	// Use context.WithoutCancel so that if the first caller's context is
	// cancelled, other callers sharing this singleflight call are not affected.
	result, err, shared := c.group.Do(cacheKey, func() (any, error) {
		return c.Backend.Get(context.WithoutCancel(ctx), id)
	})
	if err != nil {
		var zero S
		return zero, err
	}
	item := result.(S)

	if shared {
		metrics.RecordSingleflightDedup("get")
	}

	if len(item.Value()) == 0 {
		// Do not cache empty secrets
		return item, nil
	}

	cost := int64(len(item.Value())) + int64(len(cacheKey))
	added := c.Cache.SetWithTTL(cacheKey, item, cost, c.ttl)
	if !added {
		log.Info("Failed to add item to cache", "id", cacheKey)
	}
	// Always return a copy since singleflight shares the result across callers
	return item.Copy().(S), nil
}

// ParseSecretId implements backend.Backend.
func (c *CachedBackend[T, S]) ParseSecretId(raw string) (T, error) {
	return c.Backend.ParseSecretId(raw)
}

// Set implements backend.Backend.
func (c *CachedBackend[T, S]) Set(ctx context.Context, id T, value backend.SecretValue, opts ...backend.WriteOption) (S, error) {
	log := logr.FromContextOrDiscard(ctx)

	cacheId := id.Copy()
	cacheValue := value.Copy()
	cacheKey := cacheId.CacheKey()

	var res S
	if cacheValue.IsEmpty() {
		// Do not cache empty secrets, but ensure they are deleted from the cache
		metrics.RecordCacheMiss("set", "empty_value")
		c.Cache.Del(cacheKey)
		return res, backend.ErrEmptySecretValue(cacheId.(T))
	}

	cachedItem, ok := c.Cache.Get(cacheKey)
	if ok && cacheValue.EqualString(cachedItem.Value()) {
		metrics.RecordCacheHit("set", "")
		return cachedItem.Copy().(S), nil
	} else if ok {
		metrics.RecordCacheMiss("set", "value_mismatch")
		c.Cache.Del(cacheKey)
	}

	metrics.RecordCacheMiss("set", "not_found")
	item, err := c.Backend.Set(ctx, cacheId.(T), cacheValue, opts...)
	if err != nil {
		return item, err
	}
	copy := item.Copy().(S)

	cost := int64(len(value.Value())) + int64(len(cacheKey))
	added := c.Cache.SetWithTTL(copy.Id().CacheKey(), copy, cost, c.ttl)
	if !added {
		log.Info("Failed to add item to cache", "id", cacheKey)
	}
	c.group.Forget(cacheKey)
	c.invalidateParent(id)

	return item, nil
}
