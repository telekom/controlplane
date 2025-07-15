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
	if item, ok := c.Cache.Get(id.String()); ok {
		if !item.Expired() {
			metrics.Collection.RecordCacheHit()
			return item.Value(), nil
		} else {
			metrics.Collection.RecordCacheMiss("expired")
		}
	} else {
		metrics.Collection.RecordCacheMiss("not_found")
	}

	log.Info("Cache miss", "id", id.String())
	item, err := c.Backend.Get(ctx, id)
	if err != nil {
		return res, err
	}

	c.Cache.Set(id.String(), NewDefaultCacheItem(id, item, c.ttl))
	return item, nil
}

func (c *CachedBackend[T, S]) Set(ctx context.Context, id T, value backend.SecretValue) (res S, err error) {
	if item, ok := c.Cache.Get(id.String()); ok {
		if value.EqualString(item.Value().Value()) {
			return item.Value(), nil
		}
	}
	item, err := c.Backend.Set(ctx, id, value)
	if err != nil {
		return res, err
	}

	if item.Value() != "" {
		c.Cache.Set(id.String(), NewDefaultCacheItem(id, item, c.ttl))
	}
	return item, nil
}

func (c *CachedBackend[T, S]) Delete(ctx context.Context, id T) error {
	c.Cache.Delete(id.String())
	return c.Backend.Delete(ctx, id)
}
