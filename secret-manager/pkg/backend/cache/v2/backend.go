// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v2

import (
	"context"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache/metrics"
)

var _ backend.Backend[backend.SecretId, backend.Secret[backend.SecretId]] = (*CachedBackend[backend.SecretId, backend.Secret[backend.SecretId]])(nil)

type CachedBackend[T backend.SecretId, S backend.Secret[T]] struct {
	Backend backend.Backend[T, S]
	Cache   *ristretto.Cache[string, S]
	ttl     time.Duration
}

func NewCachedBackend[T backend.SecretId, S backend.Secret[T]](backend backend.Backend[T, S], ttl time.Duration) *CachedBackend[T, S] {
	cache, err := ristretto.NewCache(&ristretto.Config[string, S]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 20, // maximum cost of cache (1MB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		panic(err)
	}

	return &CachedBackend[T, S]{
		Backend: backend,
		Cache:   cache,
		ttl:     ttl,
	}
}

// Delete implements backend.Backend.
func (c *CachedBackend[T, S]) Delete(ctx context.Context, id T) error {
	c.Cache.Del(id.String())
	return c.Backend.Delete(ctx, id)
}

// Get implements backend.Backend.
func (c *CachedBackend[T, S]) Get(ctx context.Context, id T) (S, error) {
	log := logr.FromContextOrDiscard(ctx)
	cachedItem, ok := c.Cache.Get(id.String())
	if ok {
		metrics.RecordCacheHit()
		return cachedItem, nil
	}
	metrics.RecordCacheMiss("not_found")
	item, err := c.Backend.Get(ctx, id)
	if err != nil {
		return item, err
	}
	added := c.Cache.SetWithTTL(id.String(), item, 1, c.ttl)
	if !added {
		log.Info("Failed to add item to cache", "id", id.String())
	}
	return item, nil
}

// ParseSecretId implements backend.Backend.
func (c *CachedBackend[T, S]) ParseSecretId(raw string) (T, error) {
	return c.Backend.ParseSecretId(raw)
}

// Set implements backend.Backend.
func (c *CachedBackend[T, S]) Set(ctx context.Context, id T, value backend.SecretValue) (S, error) {
	log := logr.FromContextOrDiscard(ctx)

	cachedItem, ok := c.Cache.Get(id.String())
	if ok && value.EqualString(cachedItem.Value()) {
		metrics.RecordCacheHit()
		return cachedItem, nil
	} else if ok {
		metrics.RecordCacheMiss("value_mismatch")
	}

	item, err := c.Backend.Set(ctx, id, value)
	if err != nil {
		return item, err
	}
	added := c.Cache.SetWithTTL(id.String(), item, 1, c.ttl)
	if !added {
		log.Info("Failed to add item to cache", "id", id.String())
	}
	return item, nil
}
