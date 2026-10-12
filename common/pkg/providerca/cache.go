// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package providerca

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"
)

const (
	cacheSuccessTTL = time.Minute
	cacheFailureTTL = 5 * time.Second
	cacheMaxEntries = 256
	cacheStripes    = 32
)

type cacheKey struct {
	namespace, name, key string
}

type cacheEntry[B any] struct {
	key     cacheKey
	value   *Resolved[B]
	err     error
	expires time.Time
}

// Cache is a bounded, process-local cache of resolved provider CA references.
// Successful resolutions are reused for one minute after the read completed, failures
// for five seconds. Caller cancellation and deadline failures are not cached.
// Returned bundles are shared and must be treated as immutable.
type Cache[B any] struct {
	reader client.Reader
	parse  ParseFunc[B]

	now        func() time.Time
	successTTL time.Duration
	failureTTL time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[cacheKey]*list.Element
	lru     *list.List

	stripes [cacheStripes]sync.Mutex
}

// CacheOption configures a Cache.
type CacheOption func(*cacheOptions)

type cacheOptions struct {
	now func() time.Time
}

// WithClock replaces the wall clock used for expiry, e.g. to expire entries in tests.
func WithClock(now func() time.Time) CacheOption {
	return func(o *cacheOptions) { o.now = now }
}

// NewCache returns a Cache that resolves references with reader and parse. Controllers pass
// the manager's informer-backed client, whose ConfigMap cache holds only provider-ca labeled
// objects (see client.ProviderCACacheOptions). Both are required.
func NewCache[B any](reader client.Reader, parse ParseFunc[B], opts ...CacheOption) *Cache[B] {
	if reader == nil {
		panic("providerca.NewCache: reader is nil")
	}
	if parse == nil {
		panic("providerca.NewCache: parse is nil")
	}
	o := cacheOptions{now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}
	return &Cache[B]{
		reader:     reader,
		parse:      parse,
		now:        o.now,
		successTTL: cacheSuccessTTL,
		failureTTL: cacheFailureTTL,
		maxEntries: cacheMaxEntries,
		entries:    map[cacheKey]*list.Element{},
		lru:        list.New(),
	}
}

// Resolve returns the resolution of ref in namespace, reading and parsing it with Resolve
// when no unexpired entry exists. A nil ref selects system trust and returns nil without
// reading. Each successful result is a fresh copy whose metadata may be modified freely.
func (c *Cache[B]) Resolve(ctx context.Context, namespace string, ref *types.CABundleConfigMapRef) (*Resolved[B], error) {
	if ref == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := cacheKey{namespace: namespace, name: ref.Name, key: ref.Key}
	stripe := &c.stripes[stripeIndex(key.namespace, key.name, key.key)]
	stripe.Lock()
	defer stripe.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if e := c.lookup(key); e != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return copyResolved(e.value), e.err
	}

	value, err := Resolve(ctx, c.reader, namespace, ref, c.parse)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	ttl := c.successTTL
	if err != nil {
		ttl = c.failureTTL
	}
	c.store(&cacheEntry[B]{key: key, value: value, err: err, expires: c.now().Add(ttl)})
	return copyResolved(value), err
}

func (c *Cache[B]) lookup(key cacheKey) *cacheEntry[B] {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return nil
	}
	e := cacheEntryOf[B](el)
	if !c.now().Before(e.expires) {
		c.lru.Remove(el)
		delete(c.entries, key)
		return nil
	}
	c.lru.MoveToFront(el)
	return e
}

func (c *Cache[B]) store(e *cacheEntry[B]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.entries[e.key]; ok {
		c.lru.Remove(old)
	}
	c.entries[e.key] = c.lru.PushFront(e)
	for c.lru.Len() > c.maxEntries {
		oldest := c.lru.Back()
		c.lru.Remove(oldest)
		delete(c.entries, cacheEntryOf[B](oldest).key)
	}
}

func cacheEntryOf[B any](el *list.Element) *cacheEntry[B] {
	e, ok := el.Value.(*cacheEntry[B])
	if !ok {
		panic("providerca.Cache: unexpected LRU element type")
	}
	return e
}

func copyResolved[B any](r *Resolved[B]) *Resolved[B] {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

// stripeIndex hashes parts with FNV-1a, separating them by a zero byte.
func stripeIndex(parts ...string) uint32 {
	h := uint32(2166136261)
	for _, s := range parts {
		for i := 0; i < len(s); i++ {
			h = (h ^ uint32(s[i])) * 16777619
		}
		h *= 16777619
	}
	return h % cacheStripes
}
