// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"
	"time"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

var _ Cache[backend.SecretId, backend.Secret[backend.SecretId]] = (*SimpleCache[backend.SecretId, backend.Secret[backend.SecretId]])(nil)

type SimpleCache[T backend.SecretId, S backend.Secret[T]] struct {
	lock sync.RWMutex
	m    map[string]CacheItem[T, S]
}

func NewSimpleCache[T backend.SecretId, S backend.Secret[T]]() Cache[T, S] {
	return &SimpleCache[T, S]{
		m: make(map[string]CacheItem[T, S]),
	}
}

func (c *SimpleCache[T, S]) Get(id string) (CacheItem[T, S], bool) {
	c.lock.RLock()
	item, ok := c.m[id]
	c.lock.RUnlock()

	if !ok {
		return nil, false
	}
	if item.Expired() {
		c.lock.Lock()
		delete(c.m, id)
		c.lock.Unlock()
		return nil, false
	}
	return item, true
}

func (c *SimpleCache[T, S]) Delete(id string) {
	c.lock.Lock()
	delete(c.m, id)
	c.lock.Unlock()
}

func (c *SimpleCache[T, S]) Set(id string, item CacheItem[T, S]) {
	c.lock.Lock()
	c.m[id] = item
	c.lock.Unlock()
}

type CacheItem[T backend.SecretId, S backend.Secret[T]] interface {
	Id() T
	Value() S
	Expired() bool
}

type DefaultCacheItem[T backend.SecretId, S backend.Secret[T]] struct {
	id        T
	value     S
	expiresAt int64
}

func NewDefaultCacheItem[T backend.SecretId, S backend.Secret[T]](id T, value S, ttl int64) DefaultCacheItem[T, S] {
	return DefaultCacheItem[T, S]{
		id:        id,
		value:     value,
		expiresAt: time.Now().Unix() + ttl,
	}
}

func (c DefaultCacheItem[T, S]) Id() T {
	return c.id
}

func (c DefaultCacheItem[T, S]) Value() S {
	return c.value
}

func (c DefaultCacheItem[T, S]) Expired() bool {
	return time.Now().Unix() > c.expiresAt
}
