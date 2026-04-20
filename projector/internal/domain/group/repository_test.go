// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package group_test

import (
	"context"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	"github.com/telekom/controlplane/projector/internal/domain/group"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
)

var _ = Describe("Group Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		repo   *group.Repository
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		repo = group.NewRepository(client, cache)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create a new group with all fields", func() {
			data := &group.GroupData{
				Meta:        shared.NewMetadata("org", "group-a", nil),
				Name:        "group-a",
				DisplayName: "Group A",
				Description: "First group",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			g, err := client.Group.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(g.Name).To(Equal("group-a"))
			Expect(g.DisplayName).To(Equal("Group A"))
			Expect(g.Description).To(Equal("First group"))
		})

		It("should create a group with empty description", func() {
			data := &group.GroupData{
				Meta:        shared.NewMetadata("org", "group-b", nil),
				Name:        "group-b",
				DisplayName: "Group B",
				Description: "",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			g, err := client.Group.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(g.Name).To(Equal("group-b"))
			Expect(g.DisplayName).To(Equal("Group B"))
			Expect(g.Description).To(Equal(""))
		})

		It("should update existing group on conflict (idempotent)", func() {
			data := &group.GroupData{
				Meta:        shared.NewMetadata("org", "group-c", nil),
				Name:        "group-c",
				DisplayName: "Original Name",
				Description: "Original description",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update with different values.
			data.DisplayName = "Updated Name"
			data.Description = "Updated description"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			g, err := client.Group.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(g.DisplayName).To(Equal("Updated Name"))
			Expect(g.Description).To(Equal("Updated description"))
		})

		It("should populate the edge cache after upsert", func() {
			data := &group.GroupData{
				Meta:        shared.NewMetadata("org", "cached-group", nil),
				Name:        "cached-group",
				DisplayName: "Cached Group",
				Description: "For cache test",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait() // ensure async set is applied

			id, found := cache.Get("group", "cached-group")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing group", func() {
			data := &group.GroupData{
				Meta:        shared.NewMetadata("org", "del-group", nil),
				Name:        "del-group",
				DisplayName: "Delete Me",
				Description: "",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			Expect(repo.Delete(ctx, "del-group")).To(Succeed())

			count, err := client.Group.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should be idempotent for non-existent group", func() {
			Expect(repo.Delete(ctx, "nonexistent")).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &group.GroupData{
				Meta:        shared.NewMetadata("org", "evict-group", nil),
				Name:        "evict-group",
				DisplayName: "Evict Me",
				Description: "",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("group", "evict-group")
			Expect(found).To(BeTrue())

			Expect(repo.Delete(ctx, "evict-group")).To(Succeed())
			cache.Wait()

			_, found = cache.Get("group", "evict-group")
			Expect(found).To(BeFalse())
		})
	})
})
