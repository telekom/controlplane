// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	entgroup "github.com/telekom/controlplane/controlplane-api/ent/group"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
)

// IDResolverOption configures optional IDResolver behaviour.
type IDResolverOption func(*IDResolver)

// WithNegativeCacheTTL sets the TTL for negative cache entries ("entity not
// found" results). Default: 5 s. Set to 0 to effectively disable negative
// caching.
func WithNegativeCacheTTL(d time.Duration) IDResolverOption {
	return func(r *IDResolver) { r.negTTL = d }
}

// WithSingleflight enables or disables singleflight request coalescing on
// cache-miss paths. Default: true (enabled).
func WithSingleflight(enabled bool) IDResolverOption {
	return func(r *IDResolver) { r.sfEnabled = enabled }
}

// WithNowFunc overrides the clock used by the negative cache.
// This is primarily useful in tests to control time-based expiry.
func WithNowFunc(fn func() time.Time) IDResolverOption {
	return func(r *IDResolver) { r.nowFunc = fn }
}

// IDResolver performs cache-first, DB-fallback lookups for entity primary keys.
// It satisfies the narrow dependency interfaces declared by domain repositories
// (e.g., ApplicationDeps, APIExposureDeps, APISubscriptionDeps).
//
// Two hardening mechanisms protect the DB from stampedes:
//
//   - Singleflight (golang.org/x/sync/singleflight): coalesces concurrent
//     identical cache-miss lookups so that at most one DB query is executed per
//     key at any time. Controlled by sfEnabled.
//
//   - Negative caching (sync.Map with expiry): caches "entity not found"
//     results for a configurable TTL so that repeated lookups for a missing
//     dependency return immediately without touching the DB.
type IDResolver struct {
	client    *ent.Client
	cache     *EdgeCache
	sf        singleflight.Group
	sfEnabled bool
	negCache  sync.Map // map[string]time.Time (expiry timestamp)
	negTTL    time.Duration
	nowFunc   func() time.Time
}

// NewIDResolver creates an IDResolver backed by the given ent client and edge
// cache. Use [WithNegativeCacheTTL], [WithSingleflight], and [WithNowFunc] to
// override the defaults (5 s neg-TTL, singleflight enabled, real clock).
func NewIDResolver(client *ent.Client, cache *EdgeCache, opts ...IDResolverOption) *IDResolver {
	r := &IDResolver{
		client:    client,
		cache:     cache,
		negTTL:    5 * time.Second,
		sfEnabled: true,
		nowFunc:   time.Now,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// isNegCached reports whether key has a non-expired negative cache entry.
func (r *IDResolver) isNegCached(key string) bool {
	v, ok := r.negCache.Load(key)
	if !ok {
		return false
	}
	expiry, _ := v.(time.Time)
	if r.nowFunc().Before(expiry) {
		return true
	}
	// Expired — evict lazily.
	r.negCache.Delete(key)
	return false
}

// setNegCache stores a negative cache entry that expires at now + negTTL.
func (r *IDResolver) setNegCache(key string) {
	if r.negTTL <= 0 {
		return
	}
	r.negCache.Store(key, r.nowFunc().Add(r.negTTL))
}

// clearNegCache removes a negative cache entry (eager eviction on positive hit).
func (r *IDResolver) clearNegCache(key string) {
	r.negCache.Delete(key)
}

// resolve is the common lookup pipeline shared by all DB-backed Find* methods.
//
// Flow:
//  1. Edge cache hit → clear any stale neg entry, return cached ID.
//  2. Negative cache hit (not expired) → return ErrEntityNotFound immediately.
//  3. Cache miss → execute dbQuery, optionally wrapped in singleflight.
//
// The edge cache is checked first because it represents positive evidence
// ("entity exists") populated by repository Upsert calls. This takes priority
// over negative cache entries ("entity was missing N seconds ago") which may
// be stale — e.g. when a dependency is synced by another controller while a
// negative cache entry is still within its TTL.
//
// When singleflight is enabled, DoChan is used instead of Do so that waiting
// goroutines can bail out when their context expires (e.g. via the per-reconcile
// ReconciliationTimeout). The in-flight DB query continues to completion for
// the benefit of other waiters.
//
// The dbQuery function is responsible for DB interaction, populating the edge
// cache on hit, and managing neg cache entries (setNegCache on miss,
// clearNegCache on hit).
func (r *IDResolver) resolve(ctx context.Context, et, lk, label string, dbQuery func() (int, error)) (int, error) {
	fullKey := et + ":" + lk

	// Step 1: edge cache (positive evidence takes priority)
	if id, ok := r.cache.Get(et, lk); ok {
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultCacheHit).Inc()
		return id, nil
	}

	// Step 2: negative cache (only when edge cache has no entry)
	if r.isNegCached(fullKey) {
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultNegCacheHit).Inc()
		return 0, fmt.Errorf("%s: %w", label, ErrEntityNotFound)
	}

	// Step 3: DB query (with optional singleflight)
	if r.sfEnabled {
		ch := r.sf.DoChan(fullKey, func() (any, error) {
			return dbQuery()
		})
		select {
		case res := <-ch:
			metrics.IDResolverSingleflight.WithLabelValues(et, strconv.FormatBool(res.Shared)).Inc()
			if res.Err != nil {
				return 0, res.Err
			}
			return res.Val.(int), nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return dbQuery()
}

// FindZoneID looks up the DB primary key for a Zone by name.
// Returns ErrEntityNotFound (wrapped) if no matching row exists.
func (r *IDResolver) FindZoneID(ctx context.Context, name string) (int, error) {
	et, lk := cachekeys.Zone(name)
	fullKey := et + ":" + lk
	return r.resolve(ctx, et, lk, fmt.Sprintf("zone %q", name), func() (int, error) {
		z, err := r.client.Zone.Query().
			Where(zone.NameEQ(name)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				r.setNegCache(fullKey)
				metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBMiss).Inc()
				return 0, fmt.Errorf("zone %q: %w", name, ErrEntityNotFound)
			}
			return 0, fmt.Errorf("find zone %q: %w", name, err)
		}
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBHit).Inc()
		r.cache.Set(et, lk, z.ID)
		return z.ID, nil
	})
}

// FindGroupID looks up the DB primary key for a Group by name.
// Returns ErrEntityNotFound (wrapped) if no matching row exists.
func (r *IDResolver) FindGroupID(ctx context.Context, name string) (int, error) {
	et, lk := cachekeys.Group(name)
	fullKey := et + ":" + lk
	return r.resolve(ctx, et, lk, fmt.Sprintf("group %q", name), func() (int, error) {
		g, err := r.client.Group.Query().
			Where(entgroup.NameEQ(name)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				r.setNegCache(fullKey)
				metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBMiss).Inc()
				return 0, fmt.Errorf("group %q: %w", name, ErrEntityNotFound)
			}
			return 0, fmt.Errorf("find group %q: %w", name, err)
		}
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBHit).Inc()
		r.cache.Set(et, lk, g.ID)
		return g.ID, nil
	})
}

// FindTeamID looks up the DB primary key for a Team by name.
// Returns ErrEntityNotFound (wrapped) if no matching row exists.
func (r *IDResolver) FindTeamID(ctx context.Context, name string) (int, error) {
	et, lk := cachekeys.Team(name)
	fullKey := et + ":" + lk
	return r.resolve(ctx, et, lk, fmt.Sprintf("team %q", name), func() (int, error) {
		t, err := r.client.Team.Query().
			Where(team.NameEQ(name)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				r.setNegCache(fullKey)
				metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBMiss).Inc()
				return 0, fmt.Errorf("team %q: %w", name, ErrEntityNotFound)
			}
			return 0, fmt.Errorf("find team %q: %w", name, err)
		}
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBHit).Inc()
		r.cache.Set(et, lk, t.ID)
		return t.ID, nil
	})
}

// FindApplicationID looks up the DB primary key for an Application by name
// and team name. Application names are only unique per team (composite unique
// index on name + owner_team), so both are required.
// Returns ErrEntityNotFound (wrapped) if no matching row exists.
func (r *IDResolver) FindApplicationID(ctx context.Context, name, teamName string) (int, error) {
	et, lk := cachekeys.Application(name, teamName)
	fullKey := et + ":" + lk
	return r.resolve(ctx, et, lk, fmt.Sprintf("application %q (team %q)", name, teamName), func() (int, error) {
		a, err := r.client.Application.Query().
			Where(
				application.NameEQ(name),
				application.HasOwnerTeamWith(team.NameEQ(teamName)),
			).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				r.setNegCache(fullKey)
				metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBMiss).Inc()
				return 0, fmt.Errorf("application %q (team %q): %w", name, teamName, ErrEntityNotFound)
			}
			return 0, fmt.Errorf("find application %q (team %q): %w", name, teamName, err)
		}
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBHit).Inc()
		r.cache.Set(et, lk, a.ID)
		return a.ID, nil
	})
}

// FindAPIExposureID looks up the DB primary key for an ApiExposure by base
// path, application name, and team name. Base paths are unique per application,
// and applications per team, so all three are required.
// Returns ErrEntityNotFound (wrapped) if no matching row exists.
func (r *IDResolver) FindAPIExposureID(ctx context.Context, basePath, appName, teamName string) (int, error) {
	et, lk := cachekeys.APIExposure(basePath, appName, teamName)
	fullKey := et + ":" + lk
	return r.resolve(ctx, et, lk, fmt.Sprintf("api_exposure %q (app %q, team %q)", basePath, appName, teamName), func() (int, error) {
		exposure, err := r.client.ApiExposure.Query().
			Where(
				apiexposure.BasePathEQ(basePath),
				apiexposure.HasOwnerWith(
					application.NameEQ(appName),
					application.HasOwnerTeamWith(team.NameEQ(teamName)),
				),
			).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				r.setNegCache(fullKey)
				metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBMiss).Inc()
				return 0, fmt.Errorf("api_exposure %q (app %q, team %q): %w", basePath, appName, teamName, ErrEntityNotFound)
			}
			return 0, fmt.Errorf("find api_exposure %q (app %q, team %q): %w", basePath, appName, teamName, err)
		}
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBHit).Inc()
		r.cache.Set(et, lk, exposure.ID)
		return exposure.ID, nil
	})
}

// FindAPIExposureByBasePath looks up the DB primary key for an ApiExposure by
// base path alone (without requiring the owning application or team name).
// This is needed by ApiSubscription because the CR does not carry target
// app/team information. Only active exposures are matched — multiple exposures
// may share the same base path, but only one can be active at a time.
//
// Uses a separate cache key prefix ("bp:") to avoid collisions with the
// full (basePath, appName, teamName) composite key used by FindAPIExposureID.
// Returns ErrEntityNotFound (wrapped) if no matching active row exists.
func (r *IDResolver) FindAPIExposureByBasePath(ctx context.Context, basePath string) (int, error) {
	et, lk := cachekeys.APIExposureByBasePath(basePath)
	fullKey := et + ":" + lk
	return r.resolve(ctx, et, lk, fmt.Sprintf("api_exposure basePath %q", basePath), func() (int, error) {
		exposure, err := r.client.ApiExposure.Query().
			Where(apiexposure.BasePathEQ(basePath)).
			Where(apiexposure.ActiveEQ(true)).
			First(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				r.setNegCache(fullKey)
				metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBMiss).Inc()
				return 0, fmt.Errorf("api_exposure basePath %q: %w", basePath, ErrEntityNotFound)
			}
			return 0, fmt.Errorf("find api_exposure basePath %q: %w", basePath, err)
		}
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBHit).Inc()
		r.cache.Set(et, lk, exposure.ID)
		return exposure.ID, nil
	})
}

// FindAPISubscriptionByMeta looks up the DB primary key for an ApiSubscription
// by its Kubernetes metadata (namespace + name). The (namespace, name) pair
// forms a unique composite index, so at most one row can match.
// Returns ErrEntityNotFound (wrapped) if no matching row exists.
func (r *IDResolver) FindAPISubscriptionByMeta(ctx context.Context, namespace, name string) (int, error) {
	et, lk := cachekeys.APISubscriptionMeta(namespace, name)
	fullKey := et + ":" + lk
	return r.resolve(ctx, et, lk, fmt.Sprintf("api_subscription %s/%s", namespace, name), func() (int, error) {
		sub, err := r.client.ApiSubscription.Query().
			Where(
				apisubscription.NamespaceEQ(namespace),
				apisubscription.NameEQ(name),
			).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				r.setNegCache(fullKey)
				metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBMiss).Inc()
				return 0, fmt.Errorf("api_subscription %s/%s: %w", namespace, name, ErrEntityNotFound)
			}
			return 0, fmt.Errorf("find api_subscription %s/%s: %w", namespace, name, err)
		}
		r.clearNegCache(fullKey)
		metrics.IDResolverLookups.WithLabelValues(et, metrics.ResultDBHit).Inc()
		r.cache.Set(et, lk, sub.ID)
		return sub.ID, nil
	})
}
