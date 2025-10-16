// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"runtime"
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
		Cache:   NewShardedCache[T, S](uint8(runtime.NumCPU())),
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
		metrics.RecordCacheHit("")
		return item.Value().Copy().(S), nil
	}

	metrics.RecordCacheMiss("not_found")
	item, err := c.Backend.Get(ctx, id)
	if err != nil {
		return res, err
	}
	copy := item.Copy().(S)

	log.V(1).Info("Caching Get result", "id", copy.Id().String())
	c.Cache.Set(cacheKey, NewDefaultCacheItem(copy.Id(), copy, c.ttl))

	return item, nil
}

func (c *CachedBackend[T, S]) Set(ctx context.Context, id T, value backend.SecretValue) (res S, err error) {
	cacheId := id.Copy()

	if item, ok := c.Cache.Get(cacheId.String()); ok && !item.Expired() {
		cachedSecret := item.Value()
		if value.EqualString(cachedSecret.Value()) {
			metrics.RecordCacheHit("set")
			return cachedSecret.Copy().(S), nil
		} else {
			metrics.RecordCacheMiss("value_mismatch")
			c.Cache.Delete(cacheId.String())
		}
	}

	metrics.RecordCacheMiss("set")
	item, err := c.Backend.Set(ctx, cacheId.(T), value)
	if err != nil {
		return res, err
	}

	copy := item.Copy().(S)

	c.Cache.Set(copy.Id().String(), NewDefaultCacheItem(copy.Id(), copy, c.ttl))

	return item, nil
}

func (c *CachedBackend[T, S]) Delete(ctx context.Context, id T) error {
	c.Cache.Delete(id.String())
	return c.Backend.Delete(ctx, id)
}
