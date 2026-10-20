// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tlsclient

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultIdleTTL is the time since the last acquisition after which an
	// unborrowed client is evicted.
	DefaultIdleTTL = 30 * time.Minute
	// DefaultCleanupInterval is the interval of the eviction loop.
	DefaultCleanupInterval = time.Minute
)

// FactoryOption configures a Factory.
type FactoryOption func(*Factory)

// WithIdleTTL overrides DefaultIdleTTL.
func WithIdleTTL(ttl time.Duration) FactoryOption {
	return func(f *Factory) { f.idleTTL = ttl }
}

// WithCleanupInterval overrides DefaultCleanupInterval.
func WithCleanupInterval(interval time.Duration) FactoryOption {
	return func(f *Factory) { f.interval = interval }
}

// WithClock overrides time.Now.
func WithClock(now func() time.Time) FactoryOption {
	return func(f *Factory) { f.now = now }
}

type cacheKey struct {
	kind, namespace, uid, fingerprint string
}

type entry struct {
	ready        chan struct{}
	client       *Client
	err          error
	refs         int
	lastAcquired time.Time
}

func (e *entry) isReady() bool {
	select {
	case <-e.ready:
		return true
	default:
		return false
	}
}

// Factory caches clients per owner and fingerprint. It is safe for concurrent
// use and is meant to be shared by all controllers of an operator.
type Factory struct {
	ctx      context.Context
	cancel   context.CancelFunc
	profiles map[string]Profile
	idleTTL  time.Duration
	interval time.Duration
	now      func() time.Time

	mu      sync.Mutex
	closed  bool
	entries map[cacheKey]*entry
}

// NewFactory creates a factory. ctx is the operator lifetime; token sources
// of cached clients live until it is canceled or Close is called.
func NewFactory(ctx context.Context, profiles []Profile, opts ...FactoryOption) (*Factory, error) {
	f := &Factory{
		profiles: map[string]Profile{},
		idleTTL:  DefaultIdleTTL,
		interval: DefaultCleanupInterval,
		now:      time.Now,
		entries:  map[cacheKey]*entry{},
	}
	for _, p := range profiles {
		if err := p.validate(); err != nil {
			return nil, err
		}
		if _, ok := f.profiles[p.Name]; ok {
			return nil, fmt.Errorf("duplicate profile %q", p.Name)
		}
		f.profiles[p.Name] = p
	}
	for _, opt := range opts {
		opt(f)
	}
	if f.idleTTL <= 0 || f.interval <= 0 {
		return nil, fmt.Errorf("idle ttl and cleanup interval must be positive")
	}
	f.ctx, f.cancel = context.WithCancel(ctx)
	return f, nil
}

// Lease grants use of a cached client until Release is called.
type Lease struct {
	f    *Factory
	e    *entry
	once sync.Once
}

// Client returns the leased client. It must not be used after Release.
func (l *Lease) Client() *Client {
	return l.e.client
}

// Release returns the lease. It is safe to call multiple times.
func (l *Lease) Release() {
	l.once.Do(func() { l.f.release(l.e) })
}

// Acquire resolves the CA bundle with ctx and returns a lease for a client
// matching spec. The CA is resolved on every call before any cache lookup, so
// resolution errors are never masked by a cached client. Construction uses
// the factory lifetime, ctx only bounds waiting.
func (f *Factory) Acquire(ctx context.Context, spec Spec) (*Lease, error) {
	profile, ok := f.profiles[spec.Profile]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", spec.Profile)
	}
	if err := spec.Owner.validate(); err != nil {
		return nil, err
	}
	if err := spec.validate(); err != nil {
		return nil, err
	}
	caPEM, err := resolveCA(ctx, spec.CA)
	if err != nil {
		return nil, err
	}
	key := cacheKey{
		kind:        spec.Owner.Kind,
		namespace:   spec.Owner.Namespace,
		uid:         spec.Owner.UID,
		fingerprint: fingerprint(profile.Name, spec, caPEM),
	}

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil, ErrFactoryClosed
	}
	e, ok := f.entries[key]
	if !ok {
		e = &entry{ready: make(chan struct{})}
		f.entries[key] = e
		go f.build(key, e, profile, spec, caPEM)
	}
	e.refs++
	e.lastAcquired = f.now()
	f.mu.Unlock()

	select {
	case <-e.ready:
	case <-ctx.Done():
		f.release(e)
		return nil, ctx.Err()
	}
	if e.err != nil {
		f.release(e)
		return nil, e.err
	}
	return &Lease{f: f, e: e}, nil
}

func (f *Factory) build(key cacheKey, e *entry, p Profile, s Spec, caPEM []byte) {
	c, err := build(f.ctx, p, s, caPEM, false)

	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil && f.closed {
		c.Close()
		err = ErrFactoryClosed
	}
	e.client, e.err = c, err
	if err != nil && f.entries[key] == e {
		delete(f.entries, key)
	}
	close(e.ready)
}

func (f *Factory) release(e *entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e.refs--
}

// evictIdle closes clients without borrowers whose last acquisition is older than the idle TTL.
func (f *Factory) evictIdle() {
	var evicted []*Client
	f.mu.Lock()
	now := f.now()
	for k, e := range f.entries {
		if e.isReady() && e.err == nil && e.refs == 0 && now.Sub(e.lastAcquired) >= f.idleTTL {
			delete(f.entries, k)
			evicted = append(evicted, e.client)
		}
	}
	f.mu.Unlock()
	for _, c := range evicted {
		c.Close()
	}
}

// Start runs the eviction loop until ctx or the factory lifetime ends and
// closes the factory afterwards. It implements manager.Runnable.
func (f *Factory) Start(ctx context.Context) error {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	defer f.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-f.ctx.Done():
			return nil
		case <-ticker.C:
			f.evictIdle()
		}
	}
}

// NeedLeaderElection lets the eviction loop run on every replica.
func (f *Factory) NeedLeaderElection() bool {
	return false
}

// Close stops all token sources and closes all cached clients.
// Subsequent acquisitions fail with ErrFactoryClosed.
func (f *Factory) Close() {
	var clients []*Client
	f.mu.Lock()
	f.closed = true
	for k, e := range f.entries {
		if e.isReady() && e.err == nil {
			clients = append(clients, e.client)
		}
		delete(f.entries, k)
	}
	f.mu.Unlock()
	f.cancel()
	for _, c := range clients {
		c.Close()
	}
}
