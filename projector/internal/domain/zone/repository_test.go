// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone_test

import (
	"context"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/domain/zone"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
)

var _ = Describe("Zone Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		repo   *zone.Repository
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		repo = zone.NewRepository(client, cache)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create a new zone with all fields", func() {
			data := &zone.ZoneData{
				Meta:       shared.NewMetadata("admin", "zone-a", nil),
				Name:       "zone-a",
				GatewayURL: strPtr("https://gw.example.com"),
				Visibility: "WORLD",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			z, err := client.Zone.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(z.Name).To(Equal("zone-a"))
			Expect(z.GatewayURL).NotTo(BeNil())
			Expect(*z.GatewayURL).To(Equal("https://gw.example.com"))
			Expect(string(z.Visibility)).To(Equal("WORLD"))
		})

		It("should create a zone with nil gateway URL", func() {
			data := &zone.ZoneData{
				Meta:       shared.NewMetadata("admin", "zone-b", nil),
				Name:       "zone-b",
				GatewayURL: nil,
				Visibility: "ENTERPRISE",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			z, err := client.Zone.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(z.Name).To(Equal("zone-b"))
			Expect(z.GatewayURL).To(BeNil())
			Expect(string(z.Visibility)).To(Equal("ENTERPRISE"))
		})

		It("should update existing zone on conflict (idempotent)", func() {
			data := &zone.ZoneData{
				Meta:       shared.NewMetadata("admin", "zone-c", nil),
				Name:       "zone-c",
				GatewayURL: strPtr("https://gw1.example.com"),
				Visibility: "WORLD",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update with different values.
			data.GatewayURL = strPtr("https://gw2.example.com")
			data.Visibility = "ENTERPRISE"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			z, err := client.Zone.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(*z.GatewayURL).To(Equal("https://gw2.example.com"))
			Expect(string(z.Visibility)).To(Equal("ENTERPRISE"))
		})

		It("should clear GatewayURL when updated to nil", func() {
			data := &zone.ZoneData{
				Meta:       shared.NewMetadata("admin", "zone-d", nil),
				Name:       "zone-d",
				GatewayURL: strPtr("https://gw.example.com"),
				Visibility: "WORLD",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update with nil gateway URL.
			data.GatewayURL = nil
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			z, err := client.Zone.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(z.GatewayURL).To(BeNil())
		})

		It("should populate the edge cache after upsert", func() {
			data := &zone.ZoneData{
				Meta:       shared.NewMetadata("admin", "cached-zone", nil),
				Name:       "cached-zone",
				Visibility: "ENTERPRISE",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait() // ensure async set is applied

			id, found := cache.Get("zone", "cached-zone")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing zone", func() {
			data := &zone.ZoneData{
				Meta:       shared.NewMetadata("admin", "del-zone", nil),
				Name:       "del-zone",
				Visibility: "ENTERPRISE",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			Expect(repo.Delete(ctx, "del-zone")).To(Succeed())

			count, err := client.Zone.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should be idempotent for non-existent zone", func() {
			Expect(repo.Delete(ctx, "nonexistent")).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &zone.ZoneData{
				Meta:       shared.NewMetadata("admin", "evict-zone", nil),
				Name:       "evict-zone",
				Visibility: "ENTERPRISE",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("zone", "evict-zone")
			Expect(found).To(BeTrue())

			Expect(repo.Delete(ctx, "evict-zone")).To(Succeed())
			cache.Wait()

			_, found = cache.Get("zone", "evict-zone")
			Expect(found).To(BeFalse())
		})
	})
})
