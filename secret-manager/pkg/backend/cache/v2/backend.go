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
		NumCounters: 1e7,       // number of keys to track frequency of (10M).
		MaxCost:     100 << 20, // maximum cost of cache (100MB).
		BufferItems: 64,        // number of keys per Get buffer.
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
		metrics.RecordCacheHit("get", "")
		return cachedItem.Copy().(S), nil
	}
	metrics.RecordCacheMiss("get", "not_found")
	item, err := c.Backend.Get(ctx, id)
	if err != nil {
		return item, err
	}
	if item.Value() == "" {
		// Do not cache empty secrets
		return item, nil
	}

	added := c.Cache.SetWithTTL(id.String(), item, int64(len(item.Value())), c.ttl)
	if !added {
		log.Info("Failed to add item to cache", "id", id.String())
	}
	return item.Copy().(S), nil
}

// ParseSecretId implements backend.Backend.
func (c *CachedBackend[T, S]) ParseSecretId(raw string) (T, error) {
	return c.Backend.ParseSecretId(raw)
}

// Set implements backend.Backend.
func (c *CachedBackend[T, S]) Set(ctx context.Context, id T, value backend.SecretValue) (S, error) {
	log := logr.FromContextOrDiscard(ctx)

	cacheId := id.Copy()

	cachedItem, ok := c.Cache.Get(cacheId.String())
	if ok && value.EqualString(cachedItem.Value()) {
		metrics.RecordCacheHit("set", "")
		return cachedItem.Copy().(S), nil
	} else if ok {
		metrics.RecordCacheMiss("set", "value_mismatch")
		c.Cache.Del(cacheId.String())
	}

	metrics.RecordCacheMiss("set", "not_found")
	item, err := c.Backend.Set(ctx, cacheId.(T), value)
	if err != nil {
		return item, err
	}
	var copy = item.Copy().(S)

	added := c.Cache.SetWithTTL(copy.Id().String(), copy, int64(len(item.Value())), c.ttl)
	if !added {
		log.Info("Failed to add item to cache", "id", id.String())
	}
	return item, nil
}
