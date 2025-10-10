// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache/metrics"
)

type Cache[T backend.SecretId, S backend.Secret[T]] interface {
	Get(id string) (CacheItem[T, S], bool)
	Set(id string, item CacheItem[T, S])
	Delete(id string)
}

var _ backend.Backend[backend.SecretId, backend.Secret[backend.SecretId]] = (*CachedBackend[backend.SecretId, backend.Secret[backend.SecretId]])(nil)

type CachedBackend[T backend.SecretId, S backend.Secret[T]] struct {
	Backend backend.Backend[T, S]
	Cache   Cache[T, S]
	ttl     int64
}

func NewCachedBackend[T backend.SecretId, S backend.Secret[T]](backend backend.Backend[T, S], ttl time.Duration) *CachedBackend[T, S] {
	return &CachedBackend[T, S]{
		Backend: backend,
		Cache:   NewShardedCache[T, S](16),
		ttl:     int64(ttl.Seconds()),
	}
}

func (c *CachedBackend[T, S]) ParseSecretId(raw string) (T, error) {
	return c.Backend.ParseSecretId(raw)
}

func (c *CachedBackend[T, S]) Get(ctx context.Context, id T) (res S, err error) {
	log := logr.FromContextOrDiscard(ctx)
	cacheKey := id.String()
	if item, ok := c.Cache.Get(cacheKey); ok && !item.Expired() {
		metrics.RecordCacheHit()
		log.V(1).Info("✓ Cache hit Get", "key", cacheKey)
		cachedSecret := item.Value()
		// Verify the cached secret has the correct ID
		if cachedSecret.Id().String() != cacheKey {
			log.Info("Cache corruption detected in Get, invalidating", "key", cacheKey, "cached_id", cachedSecret.Id().String())
			c.Cache.Delete(cacheKey)
			metrics.RecordCacheMiss("id_mismatch")
		} else {
			// Always return a NEW Secret with the requested ID to avoid any shared state issues
			newSecret := backend.NewDefaultSecret(id, cachedSecret.Value())
			var s S = any(newSecret).(S)
			return s, nil
		}
	}

	metrics.RecordCacheMiss("not_found")
	log.V(1).Info("Cache miss - fetching from backend", "key", cacheKey)
	item, err := c.Backend.Get(ctx, id)
	if err != nil {
		return res, err
	}

	log.V(1).Info("Caching Get result", "requested_id", id.String())
	// Create new Secret with the REQUESTED id to ensure cache key matches
	cachedSecret := backend.NewDefaultSecret(id, item.Value())
	// Type assert to S to match the generic type
	var s S = any(cachedSecret).(S)
	c.Cache.Set(cacheKey, NewDefaultCacheItem(id, s, c.ttl))
	// Return the cached secret to ensure ID consistency
	return s, nil
}

func (c *CachedBackend[T, S]) Set(ctx context.Context, id T, value backend.SecretValue) (res S, err error) {
	log := logr.FromContextOrDiscard(ctx)
	cacheKey := id.String()

	if item, ok := c.Cache.Get(cacheKey); ok && !item.Expired() {
		cachedSecret := item.Value()
		if value.EqualString(cachedSecret.Value()) {
			// Verify the cached secret has the correct ID
			if cachedSecret.Id().String() != cacheKey {
				log.Info("Cache corruption detected in Set, invalidating", "key", cacheKey, "cached_id", cachedSecret.Id().String())
				c.Cache.Delete(cacheKey)
				metrics.RecordCacheMiss("id_mismatch")
			} else {
				metrics.RecordCacheHit()
				log.V(1).Info("✓ Cache hit Set", "key", cacheKey)
				// Always return a NEW Secret with the requested ID to avoid any shared state issues
				newSecret := backend.NewDefaultSecret(id, cachedSecret.Value())
				var s S = any(newSecret).(S)
				return s, nil
			}
		} else {
			metrics.RecordCacheMiss("value_mismatch")
		}
	}

	metrics.RecordCacheMiss("not_found")
	log.V(1).Info("Calling Backend.Set", "key", cacheKey)
	item, err := c.Backend.Set(ctx, id, value)
	if err != nil {
		return res, err
	}

	log.V(1).Info("Caching Set result", "requested_id", id.String())
	// Create new Secret with the REQUESTED id to ensure cache key matches
	if item.Value() != "" {
		// Cache if backend returned a value
		cachedSecret := backend.NewDefaultSecret(id, item.Value())
		// Type assert to S to match the generic type
		var s S = any(cachedSecret).(S)
		c.Cache.Set(cacheKey, NewDefaultCacheItem(id, s, c.ttl))
		// Return the cached secret to ensure ID consistency
		return s, nil
	}
	// If backend returned empty value (after update), invalidate cache
	c.Cache.Delete(cacheKey)
	// Return backend's result with corrected ID
	cachedSecret := backend.NewDefaultSecret(id, "")
	var s S = any(cachedSecret).(S)
	return s, nil
}

func (c *CachedBackend[T, S]) Delete(ctx context.Context, id T) error {
	c.Cache.Delete(id.String())
	return c.Backend.Delete(ctx, id)
}
