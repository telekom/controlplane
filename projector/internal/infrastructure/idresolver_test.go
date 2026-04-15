// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"

	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/metrics"
)

// idResolverCase describes a single entity type's IDResolver test parameters.
type idResolverCase struct {
	entityType string
	cachedKey  string
	cachedID   int
	missingKey string
	// seedDB creates the entity in DB and returns its ID.
	seedDB func(ctx context.Context, client *ent.Client) int
	// findID calls the appropriate resolver method and returns (id, err).
	findID func(ctx context.Context, r *infrastructure.IDResolver, name string) (int, error)
}

var _ = Describe("IDResolver", func() {
	var (
		client   *ent.Client
		cache    *infrastructure.EdgeCache
		resolver *infrastructure.IDResolver
		ctx      context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		resolver = infrastructure.NewIDResolver(client, cache,
			infrastructure.WithNegativeCacheTTL(0), // disable neg cache for base tests
			infrastructure.WithSingleflight(false), // disable singleflight for base tests
		)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	// ── Zone (unique setup: requires Visibility field) ──────────────────

	Describe("FindZoneID", func() {
		It("should return cached ID on cache hit", func() {
			cache.Set("zone", "cached-zone", 42)
			cache.Wait()

			id, err := resolver.FindZoneID(ctx, "cached-zone")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(42))
		})

		It("should fall back to DB on cache miss and cache the result", func() {
			z, err := client.Zone.Create().
				SetName("db-zone").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			id, err := resolver.FindZoneID(ctx, "db-zone")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(z.ID))

			cache.Wait()
			cachedID, found := cache.Get("zone", "db-zone")
			Expect(found).To(BeTrue())
			Expect(cachedID).To(Equal(z.ID))
		})

		It("should return ErrEntityNotFound for missing zone", func() {
			_, err := resolver.FindZoneID(ctx, "missing-zone")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("missing-zone"))
		})
	})

	// ── Group & Team (table-driven to avoid dupl lint) ──────────────────

	cases := []idResolverCase{
		{
			entityType: "group",
			cachedKey:  "cached-group",
			cachedID:   99,
			missingKey: "missing-group",
			seedDB: func(ctx context.Context, c *ent.Client) int {
				g, err := c.Group.Create().SetName("db-group").SetNamespace("db-group").SetDisplayName("DB Group").Save(ctx)
				Expect(err).NotTo(HaveOccurred())
				return g.ID
			},
			findID: func(ctx context.Context, r *infrastructure.IDResolver, name string) (int, error) {
				return r.FindGroupID(ctx, name)
			},
		},
		{
			entityType: "team",
			cachedKey:  "cached-team",
			cachedID:   77,
			missingKey: "missing-team",
			seedDB: func(ctx context.Context, c *ent.Client) int {
				t, err := c.Team.Create().SetName("db-team").SetNamespace("db-team").SetEmail("db-team@example.com").Save(ctx)
				Expect(err).NotTo(HaveOccurred())
				return t.ID
			},
			findID: func(ctx context.Context, r *infrastructure.IDResolver, name string) (int, error) {
				return r.FindTeamID(ctx, name)
			},
		},
	}

	for _, tc := range cases {
		Describe("Find"+capitalize(tc.entityType)+"ID", func() {
			It("should return cached ID on cache hit", func() {
				cache.Set(tc.entityType, tc.cachedKey, tc.cachedID)
				cache.Wait()

				id, err := tc.findID(ctx, resolver, tc.cachedKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(id).To(Equal(tc.cachedID))
			})

			It("should fall back to DB on cache miss and cache the result", func() {
				dbID := tc.seedDB(ctx, client)

				id, err := tc.findID(ctx, resolver, "db-"+tc.entityType)
				Expect(err).NotTo(HaveOccurred())
				Expect(id).To(Equal(dbID))

				cache.Wait()
				cachedID, found := cache.Get(tc.entityType, "db-"+tc.entityType)
				Expect(found).To(BeTrue())
				Expect(cachedID).To(Equal(dbID))
			})

			It("should return ErrEntityNotFound for missing entity", func() {
				_, err := tc.findID(ctx, resolver, tc.missingKey)
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring(tc.missingKey))
			})
		})
	}

	// ── FindAPIExposureByBasePath (base-path-only lookup) ──────────────

	Describe("FindAPIExposureByBasePath", func() {
		It("should return cached ID on cache hit", func() {
			cache.Set("apiexposure", "bp:/api/v1/users", 55)
			cache.Wait()

			id, err := resolver.FindAPIExposureByBasePath(ctx, "/api/v1/users")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(55))
		})

		It("should fall back to DB on cache miss and cache the result", func() {
			// Seed Zone → Team → Application → ApiExposure chain.
			z, err := client.Zone.Create().
				SetName("caas").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			t, err := client.Team.Create().
				SetName("team-bp").
				SetEmail("bp@example.com").
				SetNamespace("team-bp").
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			app, err := client.Application.Create().
				SetName("app-bp").
				SetNamespace("team-bp").
				SetOwnerTeamID(t.ID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			exposure, err := client.ApiExposure.Create().
				SetBasePath("/api/v1/orders").
				SetNamespace("team-bp").
				SetVisibility("WORLD").
				SetActive(true).
				SetFeatures([]string{}).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			id, foundErr := resolver.FindAPIExposureByBasePath(ctx, "/api/v1/orders")
			Expect(foundErr).NotTo(HaveOccurred())
			Expect(id).To(Equal(exposure.ID))

			cache.Wait()
			cachedID, found := cache.Get("apiexposure", "bp:/api/v1/orders")
			Expect(found).To(BeTrue())
			Expect(cachedID).To(Equal(exposure.ID))
		})

		It("should return ErrEntityNotFound for missing base path", func() {
			_, err := resolver.FindAPIExposureByBasePath(ctx, "/api/v1/nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("/api/v1/nonexistent"))
		})

		It("should only match active exposures", func() {
			// Seed Zone → Team → Application → two ApiExposures (same basePath).
			z, err := client.Zone.Create().
				SetName("caas-active").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			t, err := client.Team.Create().
				SetName("team-active").
				SetEmail("active@example.com").
				SetNamespace("team-active").
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			app, err := client.Application.Create().
				SetName("app-active").
				SetNamespace("team-active").
				SetOwnerTeamID(t.ID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Inactive exposure — should NOT be found.
			_, err = client.ApiExposure.Create().
				SetBasePath("/api/v1/shared").
				SetNamespace("team-active").
				SetVisibility("WORLD").
				SetActive(false).
				SetFeatures([]string{}).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			// With only an inactive exposure, lookup should fail.
			_, err = resolver.FindAPIExposureByBasePath(ctx, "/api/v1/shared")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// Create a second app with an active exposure on the same basePath.
			app2, err := client.Application.Create().
				SetName("app-active-2").
				SetNamespace("team-active").
				SetOwnerTeamID(t.ID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			activeExposure, err := client.ApiExposure.Create().
				SetBasePath("/api/v1/shared").
				SetNamespace("team-active-2").
				SetVisibility("WORLD").
				SetActive(true).
				SetFeatures([]string{}).
				SetOwnerID(app2.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Now the active one should be found.
			id, err := resolver.FindAPIExposureByBasePath(ctx, "/api/v1/shared")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(activeExposure.ID))
		})
	})

	// ── FindAPISubscriptionByMeta (cache-first, DB-fallback lookup) ───

	Describe("FindAPISubscriptionByMeta", func() {
		It("should return cached ID when present", func() {
			cache.Set("apisubscription", "meta:ns:my-sub", 123)
			cache.Wait()

			id, err := resolver.FindAPISubscriptionByMeta(ctx, "ns", "my-sub")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(123))
		})

		It("should fall back to DB on cache miss and cache the result", func() {
			// Seed Zone → Team → Application → ApiSubscription chain.
			z, err := client.Zone.Create().
				SetName("caas-sub").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			t, err := client.Team.Create().
				SetName("team-sub").
				SetEmail("sub@example.com").
				SetNamespace("team-sub").
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			app, err := client.Application.Create().
				SetName("app-sub").
				SetNamespace("team-sub").
				SetOwnerTeamID(t.ID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			sub, err := client.ApiSubscription.Create().
				SetNamespace("prod--platform").
				SetName("my-subscription").
				SetBasePath("/api/v1/orders").
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			id, err := resolver.FindAPISubscriptionByMeta(ctx, "prod--platform", "my-subscription")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(sub.ID))

			cache.Wait()
			cachedID, found := cache.Get("apisubscription", "meta:prod--platform:my-subscription")
			Expect(found).To(BeTrue())
			Expect(cachedID).To(Equal(sub.ID))
		})

		It("should return ErrEntityNotFound when not in cache or DB", func() {
			_, err := resolver.FindAPISubscriptionByMeta(ctx, "ns", "missing-sub")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("ns/missing-sub"))
		})
	})
})

// ── IDResolver Hardening (Phase 4) ─────────────────────────────────────────

var _ = Describe("IDResolver Hardening", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	// ── Negative caching ──────────────────────────────────────────────

	Describe("negative caching", func() {
		It("should return neg_cache_hit on repeated lookup within TTL", func() {
			now := time.Now()
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(10*time.Second),
				infrastructure.WithSingleflight(false),
				infrastructure.WithNowFunc(func() time.Time { return now }),
			)

			// First lookup → DB miss → populates neg cache.
			_, err := resolver.FindZoneID(ctx, "no-such-zone")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// Record neg-cache-hit counter before second lookup.
			before := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultNegCacheHit),
			)

			// Second lookup within TTL → served from neg cache (no DB query).
			_, err = resolver.FindZoneID(ctx, "no-such-zone")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			after := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultNegCacheHit),
			)
			Expect(after - before).To(Equal(1.0))
		})

		It("should fall through to DB after neg cache TTL expires", func() {
			now := time.Now()
			clock := func() time.Time { return now }
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(5*time.Second),
				infrastructure.WithSingleflight(false),
				infrastructure.WithNowFunc(clock),
			)

			// First lookup → DB miss.
			_, err := resolver.FindZoneID(ctx, "expiry-zone")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// Advance past TTL.
			now = now.Add(6 * time.Second)

			dbMissBefore := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultDBMiss),
			)

			// Third lookup after TTL → should reach DB again (not neg cache).
			_, err = resolver.FindZoneID(ctx, "expiry-zone")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			dbMissAfter := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultDBMiss),
			)
			Expect(dbMissAfter - dbMissBefore).To(Equal(1.0))
		})

		It("should find entity after neg cache expires when entity is created", func() {
			now := time.Now()
			clock := func() time.Time { return now }
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(5*time.Second),
				infrastructure.WithSingleflight(false),
				infrastructure.WithNowFunc(clock),
			)

			// First lookup → DB miss.
			_, err := resolver.FindZoneID(ctx, "late-zone")
			Expect(err).To(HaveOccurred())

			// Create the zone in DB (simulates delayed sync).
			z, err := client.Zone.Create().
				SetName("late-zone").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Still within TTL → neg cache blocks the lookup.
			_, err = resolver.FindZoneID(ctx, "late-zone")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// Advance past TTL.
			now = now.Add(6 * time.Second)

			// After TTL → falls through to DB, finds the zone.
			id, err := resolver.FindZoneID(ctx, "late-zone")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(z.ID))
		})

		It("should clear neg cache entry on edge cache hit", func() {
			now := time.Now()
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(10*time.Second),
				infrastructure.WithSingleflight(false),
				infrastructure.WithNowFunc(func() time.Time { return now }),
			)

			// First lookup → DB miss → neg cache populated.
			_, err := resolver.FindZoneID(ctx, "neg-then-cached")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// Manually populate edge cache (simulates another path caching it).
			cache.Set("zone", "neg-then-cached", 999)
			cache.Wait()

			// Next lookup → edge cache hit clears neg entry, returns ID.
			id, err := resolver.FindZoneID(ctx, "neg-then-cached")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(999))
		})

		It("should resolve immediately when dependency is synced during neg cache TTL (startup scenario)", func() {
			// Reproduces the startup race: Application is reconciled before
			// Team has been synced. Team is then synced (populating the edge
			// cache) while the negative cache entry for the team is still
			// alive. The next Application reconcile should succeed immediately
			// via the edge cache without waiting for the neg cache TTL to
			// expire.
			now := time.Now()
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(30*time.Second),
				infrastructure.WithSingleflight(false),
				infrastructure.WithNowFunc(func() time.Time { return now }),
			)

			// t=0: Application controller tries to find "startup-team" → DB miss.
			_, err := resolver.FindTeamID(ctx, "startup-team")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// t=0.5s: Team controller syncs "startup-team" into DB and
			// populates the edge cache (as team/repository.Upsert does).
			t, dbErr := client.Team.Create().
				SetName("startup-team").
				SetEmail("startup@example.com").
				SetNamespace("startup-team").
				Save(ctx)
			Expect(dbErr).NotTo(HaveOccurred())

			cache.Set("team", "startup-team", t.ID)
			cache.Wait()

			// t=2s: Application controller is requeued. Neg cache TTL has
			// NOT expired (only 2s of 30s elapsed). With the correct check
			// order (edge cache before neg cache), this should succeed.
			cacheHitBefore := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("team", metrics.ResultCacheHit),
			)

			id, err := resolver.FindTeamID(ctx, "startup-team")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(t.ID))

			cacheHitAfter := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("team", metrics.ResultCacheHit),
			)
			Expect(cacheHitAfter-cacheHitBefore).To(Equal(1.0),
				"should be resolved via edge cache, not neg cache or DB")
		})

		It("should not populate neg cache when TTL is zero", func() {
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(0),
				infrastructure.WithSingleflight(false),
			)

			dbMissBefore := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultDBMiss),
			)

			// Two lookups for missing entity.
			_, _ = resolver.FindZoneID(ctx, "no-neg-zone")
			_, _ = resolver.FindZoneID(ctx, "no-neg-zone")

			dbMissAfter := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultDBMiss),
			)
			// Both should hit DB (no neg cache).
			Expect(dbMissAfter - dbMissBefore).To(Equal(2.0))
		})
	})

	// ── Singleflight ──────────────────────────────────────────────────

	Describe("singleflight", func() {
		It("should coalesce concurrent lookups for the same missing entity", func() {
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(0),
				infrastructure.WithSingleflight(true),
			)

			sharedBefore := testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "true"),
			)
			ownerBefore := testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "false"),
			)

			const n = 10
			var wg sync.WaitGroup
			wg.Add(n)
			errs := make([]error, n)
			for i := range n {
				go func(idx int) {
					defer wg.Done()
					_, errs[idx] = resolver.FindZoneID(ctx, "sf-missing")
				}(i)
			}
			wg.Wait()

			// All goroutines should get the same error.
			for i := range n {
				Expect(errors.Is(errs[i], infrastructure.ErrEntityNotFound)).To(BeTrue())
			}

			sharedAfter := testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "true"),
			)
			ownerAfter := testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "false"),
			)

			totalSF := (sharedAfter - sharedBefore) + (ownerAfter - ownerBefore)
			// All n calls should go through singleflight (cache was empty).
			Expect(totalSF).To(BeNumerically("==", n))
			// At least one must be shared (coalesced).
			Expect(sharedAfter - sharedBefore).To(BeNumerically(">=", 1))
		})

		It("should coalesce concurrent lookups for the same existing entity", func() {
			z, err := client.Zone.Create().
				SetName("sf-exists").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(0),
				infrastructure.WithSingleflight(true),
			)

			const n = 10
			var wg sync.WaitGroup
			wg.Add(n)
			ids := make([]int, n)
			errs := make([]error, n)
			for i := range n {
				go func(idx int) {
					defer wg.Done()
					ids[idx], errs[idx] = resolver.FindZoneID(ctx, "sf-exists")
				}(i)
			}
			wg.Wait()

			for i := range n {
				Expect(errs[i]).NotTo(HaveOccurred())
				Expect(ids[i]).To(Equal(z.ID))
			}
		})

		It("should not use singleflight when disabled", func() {
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(0),
				infrastructure.WithSingleflight(false),
			)

			sfBefore := testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "false"),
			) + testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "true"),
			)

			_, _ = resolver.FindZoneID(ctx, "sf-disabled-zone")

			sfAfter := testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "false"),
			) + testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("zone", "true"),
			)

			// No singleflight metrics should be emitted.
			Expect(sfAfter - sfBefore).To(Equal(0.0))
		})
	})

	// ── Combined: singleflight + negative cache ───────────────────────

	Describe("singleflight + negative cache combined", func() {
		It("should coalesce concurrent lookups then serve from neg cache", func() {
			now := time.Now()
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(10*time.Second),
				infrastructure.WithSingleflight(true),
				infrastructure.WithNowFunc(func() time.Time { return now }),
			)

			// Wave 1: concurrent lookups for missing entity → singleflight.
			const n = 5
			var wg sync.WaitGroup
			wg.Add(n)
			for range n {
				go func() {
					defer wg.Done()
					_, _ = resolver.FindZoneID(ctx, "sf-neg-combined")
				}()
			}
			wg.Wait()

			// Wave 2: serial lookups → should all be neg cache hits.
			negBefore := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultNegCacheHit),
			)

			const m = 3
			for range m {
				_, err := resolver.FindZoneID(ctx, "sf-neg-combined")
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
			}

			negAfter := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultNegCacheHit),
			)
			Expect(negAfter - negBefore).To(Equal(float64(m)))
		})
	})

	// ── Context-aware singleflight (DoChan) ──────────────────────────

	Describe("context-aware singleflight", func() {
		It("should return context error when context is already cancelled", func() {
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(0),
				infrastructure.WithSingleflight(true),
			)

			// Seed a zone so the query would succeed with a valid context.
			_, err := client.Zone.Create().
				SetName("ctx-cancel-zone").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Cancel the context before making the call.
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()

			_, err = resolver.FindZoneID(cancelledCtx, "ctx-cancel-zone")
			// With an already-cancelled context, the select in DoChan
			// should take the ctx.Done() branch (or the DB query fails
			// with the cancelled context). Either way, we get a context error.
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		})

		It("should return context error when context deadline has expired", func() {
			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(0),
				infrastructure.WithSingleflight(true),
			)

			// Seed a zone.
			_, err := client.Zone.Create().
				SetName("ctx-deadline-zone").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Use an already-expired deadline.
			deadCtx, deadCancel := context.WithDeadline(ctx, time.Now().Add(-1*time.Second))
			defer deadCancel()

			_, err = resolver.FindZoneID(deadCtx, "ctx-deadline-zone")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		})

		It("should still resolve successfully with DoChan under normal conditions", func() {
			z, err := client.Zone.Create().
				SetName("dochan-normal-zone").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			resolver := infrastructure.NewIDResolver(client, cache,
				infrastructure.WithNegativeCacheTTL(0),
				infrastructure.WithSingleflight(true),
			)

			// Concurrent lookups — all should succeed.
			const n = 10
			var wg sync.WaitGroup
			wg.Add(n)
			ids := make([]int, n)
			errs := make([]error, n)
			for i := range n {
				go func(idx int) {
					defer wg.Done()
					ids[idx], errs[idx] = resolver.FindZoneID(ctx, "dochan-normal-zone")
				}(i)
			}
			wg.Wait()

			for i := range n {
				Expect(errs[i]).NotTo(HaveOccurred())
				Expect(ids[i]).To(Equal(z.ID))
			}
		})
	})
})

// capitalize returns s with the first letter upper-cased (ASCII only).
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
