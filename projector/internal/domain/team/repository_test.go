// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package team_test

import (
	"context"
	"fmt"

	"entgo.io/ent/privacy"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	entteam "github.com/telekom/controlplane/controlplane-api/ent/team"

	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/domain/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
)

// mockTeamDeps is a test double for team.TeamDeps that allows controlling
// whether FindGroupID succeeds or fails.
type mockTeamDeps struct {
	groupIDs map[string]int
}

func (m *mockTeamDeps) FindGroupID(_ context.Context, name string) (int, error) {
	if id, ok := m.groupIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("group %q: %w", name, infrastructure.ErrEntityNotFound)
}

var _ = Describe("Team Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockTeamDeps
		repo   *team.Repository
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		deps = &mockTeamDeps{groupIDs: map[string]int{}}
		repo = team.NewRepository(client, cache, deps, logr.Discard())
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create a team with group FK and members", func() {
			// Create the group in the DB first.
			g, err := client.Group.Create().
				SetName("platform").
				SetDisplayName("Platform").
				SetNamespace("platform").
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			deps.groupIDs["platform"] = g.ID

			data := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "platform--narvi", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				Name:          "platform--narvi",
				Email:         "narvi@example.com",
				Category:      "CUSTOMER",
				GroupName:     "platform",
				Members: []team.MemberData{
					{Name: "Alice", Email: "alice@example.com"},
					{Name: "Bob", Email: "bob@example.com"},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify team was created.
			t, err := client.Team.Query().Where(entteam.NameEQ("platform--narvi")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Email).To(Equal("narvi@example.com"))
			Expect(string(t.Category)).To(Equal("CUSTOMER"))
			Expect(t.StatusPhase).ToNot(BeNil())
			Expect(t.StatusPhase.String()).To(Equal("READY"))

			// Verify group edge is set.
			grp, err := t.QueryGroup().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(grp.ID).To(Equal(g.ID))

			// Verify members were created.
			members, err := t.QueryMembers().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(members).To(HaveLen(2))
		})

		It("should create a team without group FK when group is missing (optional)", func() {
			// deps has no group registered → FindGroupID will fail.
			data := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "unknown--team-a", nil),
				StatusPhase:   "UNKNOWN",
				StatusMessage: "",
				Name:          "unknown--team-a",
				Email:         "team-a@example.com",
				Category:      "CUSTOMER",
				GroupName:     "unknown",
				Members: []team.MemberData{
					{Name: "Charlie", Email: "charlie@example.com"},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify team was created (without group edge).
			t, err := client.Team.Query().Where(entteam.NameEQ("unknown--team-a")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Email).To(Equal("team-a@example.com"))

			// Verify no group edge.
			_, err = t.QueryGroup().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			// Verify member was created.
			members, err := t.QueryMembers().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(members).To(HaveLen(1))
			Expect(members[0].Name).To(Equal("Charlie"))
		})

		It("should update existing team on conflict and sync members", func() {
			data := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "grp--upd", nil),
				StatusPhase:   "READY",
				StatusMessage: "v1",
				Name:          "grp--upd",
				Email:         "old@example.com",
				Category:      "CUSTOMER",
				GroupName:     "grp",
				Members: []team.MemberData{
					{Name: "Alice", Email: "alice@example.com"},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update with different values and additional member.
			data.Email = "new@example.com"
			data.Category = "INFRASTRUCTURE"
			data.StatusMessage = "v2"
			data.Members = []team.MemberData{
				{Name: "Alice", Email: "alice-updated@example.com"},
				{Name: "Bob", Email: "bob@example.com"},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			t, err := client.Team.Query().Where(entteam.NameEQ("grp--upd")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Email).To(Equal("new@example.com"))
			Expect(string(t.Category)).To(Equal("INFRASTRUCTURE"))
			Expect(t.StatusMessage).ToNot(BeNil())
			Expect(*t.StatusMessage).To(Equal("v2"))

			// The new upsert creates fresh members and deletes orphans.
			members, err := t.QueryMembers().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(members).To(HaveLen(2))
		})

		It("should remove orphaned members on update", func() {
			data := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "grp--orphan", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "grp--orphan",
				Email:         "orphan@example.com",
				Category:      "CUSTOMER",
				GroupName:     "grp",
				Members: []team.MemberData{
					{Name: "Alice", Email: "alice@example.com"},
					{Name: "Bob", Email: "bob@example.com"},
					{Name: "Charlie", Email: "charlie@example.com"},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify 3 members.
			t, err := client.Team.Query().Where(entteam.NameEQ("grp--orphan")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			members, err := t.QueryMembers().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(members).To(HaveLen(3))

			// Update with only 1 member — 2 should be orphaned and deleted.
			data.Members = []team.MemberData{
				{Name: "Alice", Email: "alice@example.com"},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			members, err = t.QueryMembers().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			// After sync: 1 new member created, 3 old members deleted as orphans.
			// (syncMembers creates fresh members each time, then deleteOrphanedMembers
			// removes those not in the new batch.)
			Expect(members).To(HaveLen(1))
			Expect(members[0].Name).To(Equal("Alice"))
		})

		It("should populate the edge cache after upsert", func() {
			data := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "grp--cached", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "grp--cached",
				Email:         "cached@example.com",
				Category:      "CUSTOMER",
				GroupName:     "grp",
				Members:       []team.MemberData{{Name: "Dan", Email: "dan@example.com"}},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("team", "grp--cached")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete team and its members", func() {
			data := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "grp--del", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "grp--del",
				Email:         "del@example.com",
				Category:      "CUSTOMER",
				GroupName:     "grp",
				Members: []team.MemberData{
					{Name: "Alice", Email: "alice@example.com"},
					{Name: "Bob", Email: "bob@example.com"},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			Expect(repo.Delete(ctx, "grp--del")).To(Succeed())

			// Verify team is gone.
			count, err := client.Team.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			// Verify members are gone.
			memberCount, err := client.Member.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(memberCount).To(Equal(0))
		})

		It("should be idempotent for non-existent team", func() {
			Expect(repo.Delete(ctx, "nonexistent")).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "grp--evict", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "grp--evict",
				Email:         "evict@example.com",
				Category:      "CUSTOMER",
				GroupName:     "grp",
				Members:       []team.MemberData{{Name: "Eve", Email: "eve@example.com"}},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("team", "grp--evict")
			Expect(found).To(BeTrue())

			Expect(repo.Delete(ctx, "grp--evict")).To(Succeed())
			cache.Wait()

			_, found = cache.Get("team", "grp--evict")
			Expect(found).To(BeFalse())
		})

		It("should only delete members belonging to the target team", func() {
			// Create two teams with members.
			data1 := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "grp--t1", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "grp--t1",
				Email:         "t1@example.com",
				Category:      "CUSTOMER",
				GroupName:     "grp",
				Members:       []team.MemberData{{Name: "Alice", Email: "alice@example.com"}},
			}
			data2 := &team.TeamData{
				Meta:          shared.NewMetadata("prod", "grp--t2", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "grp--t2",
				Email:         "t2@example.com",
				Category:      "CUSTOMER",
				GroupName:     "grp",
				Members:       []team.MemberData{{Name: "Bob", Email: "bob@example.com"}},
			}
			Expect(repo.Upsert(ctx, data1)).To(Succeed())
			Expect(repo.Upsert(ctx, data2)).To(Succeed())

			// Delete team 1 only.
			Expect(repo.Delete(ctx, "grp--t1")).To(Succeed())

			// Team 2 and its member should still exist.
			t2, err := client.Team.Query().Where(entteam.NameEQ("grp--t2")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			members, err := t2.QueryMembers().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(members).To(HaveLen(1))
			Expect(members[0].Name).To(Equal("Bob"))

			// Total members in DB should be 1 (only Bob).
			totalMembers, err := client.Member.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(totalMembers).To(Equal(1))
		})
	})
})
