// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package providerca

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"

	ktypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/common/pkg/types"
)

const (
	clientCacheMaxEntries = 1024
	clientCacheStripes    = 32
)

// ErrClientCacheClosed is returned by acquisitions after the client cache was closed.
var ErrClientCacheClosed = errors.New("provider client cache is closed")

// Bundle is a parsed CA bundle. Digest identifies its trust anchors canonically, so
// equivalent PEM encodings do not replace clients. Nil pointers are never valid bundles.
type Bundle interface {
	comparable
	Digest() string
}

// CAResolver resolves CA references in a namespace. A nil reference resolves to nil.
// *Cache implements it.
type CAResolver[B any] interface {
	Resolve(ctx context.Context, namespace string, ref *types.CABundleConfigMapRef) (*Resolved[B], error)
}

// ClientRequest describes the current connection of one owner.
type ClientRequest struct {
	// Owner keys the cache entry; CA references resolve in its namespace.
	Owner ktypes.NamespacedName
	// UID prevents reuse across owner recreation.
	UID ktypes.UID
	// Parts are the current connection and resolved authentication inputs.
	Parts []string
	// Ref is the CA bundle trusted by all endpoints of the connection; nil selects system trust.
	Ref *types.CABundleConfigMapRef
}

// Built is a constructed client and its optional idle-connection cleanup.
type Built[T any] struct {
	Value     T
	CloseIdle func()
}

func (b Built[T]) closeIdle() {
	if b.CloseIdle != nil {
		b.CloseIdle()
	}
}

// Builder constructs a client from the bundle of ClientRequest.Ref; a zero bundle selects
// system trust. callCtx bounds initial work such as authentication;
// lifetimeCtx outlives reconciliations and is used for later token refresh.
// A failing builder releases what it created.
type Builder[T any, B Bundle] func(callCtx, lifetimeCtx context.Context, bundle B) (Built[T], error)

type clientEntry[T any] struct {
	owner       ktypes.NamespacedName
	fingerprint string
	built       Built[T]
}

// ClientCache retains one provider client per owner. A client is reused while the
// fingerprint of its owner UID, connection inputs and resolved CA identities is unchanged;
// otherwise a replacement is built and the old client's idle connections are closed.
// Acquired clients must only be used for the current reconciliation.
// ClientCache is a manager Runnable: stopping the manager closes it.
type ClientCache[T any, B Bundle] struct {
	ca         CAResolver[B]
	maxEntries int
	lifetime   context.Context
	cancel     context.CancelFunc

	mu      sync.Mutex
	closed  bool
	entries map[ktypes.NamespacedName]*list.Element
	lru     *list.List

	stripes [clientCacheStripes]sync.Mutex
}

// NewClientCache returns a ClientCache resolving trust with ca, which is required.
func NewClientCache[T any, B Bundle](ca CAResolver[B]) *ClientCache[T, B] {
	if ca == nil {
		panic("providerca.NewClientCache: CA resolver is nil")
	}
	lifetime, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is called by Close
	return &ClientCache[T, B]{
		ca:         ca,
		maxEntries: clientCacheMaxEntries,
		lifetime:   lifetime,
		cancel:     cancel,
		entries:    map[ktypes.NamespacedName]*list.Element{},
		lru:        list.New(),
	}
}

// Acquire resolves the trust of request, then returns the owner's cached client if its
// fingerprint matches, or builds and publishes a replacement. A CA resolution failure is
// returned unchanged (see AsResolveError), as is a build failure; a previous client is never
// returned instead.
//
//nolint:gocritic // hugeParam: retain a value-based request interface
func (c *ClientCache[T, B]) Acquire(ctx context.Context, request ClientRequest, build Builder[T, B]) (T, error) {
	var zero T
	if build == nil {
		return zero, errors.New("acquiring provider client: builder is nil")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if c.isClosed() {
		return zero, ErrClientCacheClosed
	}
	var bundle B
	var resolved *Resolved[B]
	if request.Ref != nil {
		r, err := c.ca.Resolve(ctx, request.Owner.Namespace, request.Ref)
		if err != nil {
			return zero, err
		}
		if r == nil || r.Bundle == bundle {
			return zero, errors.New("acquiring provider client: CA reference resolved to system trust")
		}
		resolved, bundle = r, r.Bundle
	}
	fingerprint := fingerprintOf(&request, resolved)

	stripe := &c.stripes[stripeIndex(request.Owner.Namespace, request.Owner.Name)%clientCacheStripes]
	stripe.Lock()
	defer stripe.Unlock()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if c.isClosed() {
		return zero, ErrClientCacheClosed
	}
	if cached := c.lookup(request.Owner, fingerprint); cached != nil {
		return cached.Value, nil
	}

	built, err := build(ctx, c.lifetime, bundle)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		built.closeIdle()
		return zero, err
	}
	if err := c.publish(&clientEntry[T]{owner: request.Owner, fingerprint: fingerprint, built: built}); err != nil {
		built.closeIdle()
		return zero, err
	}
	return built.Value, nil
}

// Start blocks until ctx is done and then closes the cache (manager.Runnable).
func (c *ClientCache[T, B]) Start(ctx context.Context) error {
	<-ctx.Done()
	c.Close()
	return nil
}

// NeedLeaderElection reports false so every manager closes its cache.
func (c *ClientCache[T, B]) NeedLeaderElection() bool { return false }

// Close cancels the lifetime context and closes idle connections of all retained clients.
// It is idempotent; later acquisitions fail with ErrClientCacheClosed.
func (c *ClientCache[T, B]) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.cancel()
	retired := make([]Built[T], 0, c.lru.Len())
	for el := c.lru.Front(); el != nil; el = el.Next() {
		retired = append(retired, clientEntryOf[T](el).built)
	}
	c.lru.Init()
	c.entries = map[ktypes.NamespacedName]*list.Element{}
	c.mu.Unlock()
	for _, b := range retired {
		b.closeIdle()
	}
}

func (c *ClientCache[T, B]) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// lookup returns the client of owner if its fingerprint matches, or nil.
func (c *ClientCache[T, B]) lookup(owner ktypes.NamespacedName, fingerprint string) *Built[T] {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[owner]
	if !ok {
		return nil
	}
	e := clientEntryOf[T](el)
	if e.fingerprint != fingerprint {
		return nil
	}
	c.lru.MoveToFront(el)
	return &e.built
}

// publish stores e, replacing and evicting older clients. Their idle connections are
// closed after the map lock is released.
func (c *ClientCache[T, B]) publish(e *clientEntry[T]) error {
	var retired []Built[T]
	defer func() {
		for _, b := range retired {
			b.closeIdle()
		}
	}()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClientCacheClosed
	}
	if old, ok := c.entries[e.owner]; ok {
		retired = append(retired, clientEntryOf[T](old).built)
		c.lru.Remove(old)
	}
	c.entries[e.owner] = c.lru.PushFront(e)
	for c.lru.Len() > c.maxEntries {
		oldest := c.lru.Back()
		evicted := clientEntryOf[T](oldest)
		c.lru.Remove(oldest)
		delete(c.entries, evicted.owner)
		retired = append(retired, evicted.built)
	}
	return nil
}

func clientEntryOf[T any](el *list.Element) *clientEntry[T] {
	e, ok := el.Value.(*clientEntry[T])
	if !ok {
		panic("providerca.ClientCache: unexpected LRU element type")
	}
	return e
}

// fingerprintOf is a length-framed SHA-256 of the owner, inputs and CA identity.
func fingerprintOf[B Bundle](request *ClientRequest, r *Resolved[B]) string {
	h := sha256.New()
	write := func(s string) {
		h.Write([]byte(strconv.Itoa(len(s)) + ":" + s))
	}
	write(request.Owner.Namespace)
	write(string(request.UID))
	write(strconv.Itoa(len(request.Parts)))
	for _, p := range request.Parts {
		write(p)
	}
	if r == nil {
		write("system")
		return hex.EncodeToString(h.Sum(nil))
	}
	write("ref")
	write(r.Ref.Name)
	write(r.Ref.Key)
	write(string(r.ConfigMapUID))
	write(r.Bundle.Digest())
	return hex.EncodeToString(h.Sum(nil))
}
