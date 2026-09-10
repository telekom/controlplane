// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entagentcard "github.com/telekom/controlplane/controlplane-api/ent/agentcard"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	"github.com/telekom/controlplane/projector/internal/domain/agentcard"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockAgentCardDeps implements agentcard.AgentCardDeps for testing.
type mockAgentCardDeps struct {
	teamIDs map[string]int // key: team name
	teamErr error          // if non-nil, FindTeamID always returns this error
}

func (m *mockAgentCardDeps) FindTeamID(_ context.Context, name string) (int, error) {
	if m.teamErr != nil {
		return 0, m.teamErr
	}
	if id, ok := m.teamIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("team %q: %w", name, infrastructure.ErrEntityNotFound)
}

var _ = Describe("AgentCard Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockAgentCardDeps
		repo   *agentcard.Repository
		ctx    context.Context
		teamID int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		tm, err := client.Team.Create().
			SetName("platform--narvi").
			SetEmail("narvi@example.com").
			SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		teamID = tm.ID

		deps = &mockAgentCardDeps{
			teamIDs: map[string]int{"platform--narvi": teamID},
		}
		repo = agentcard.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an agent_card with valid deps", func() {
			data := &agentcard.AgentCardData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "card-weather-v1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/agent/weather/v1",
				Version:       "1.0.0",
				Name:          "weather-agent",
				Description:   "Weather agent card",
				Category:      "g-api",
				Oauth2Scopes:  []string{"scope-a"},
				Active:        true,
				TeamName:      "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			card, err := client.AgentCard.Query().
				Where(entagentcard.BasePathEQ("/agent/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(card.BasePath).To(Equal("/agent/weather/v1"))
			Expect(card.Version).To(Equal("1.0.0"))
			Expect(card.Name).To(Equal("weather-agent"))
			Expect(card.Description).To(Equal("Weather agent card"))
			Expect(card.Category).To(Equal("g-api"))
			Expect(card.Oauth2Scopes).To(Equal([]string{"scope-a"}))
			Expect(card.Active).To(BeTrue())

			owner, err := card.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(teamID))
		})

		It("should return ErrDependencyMissing when the owner Team is missing", func() {
			deps.teamIDs = map[string]int{}
			data := &agentcard.AgentCardData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "card-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/agent/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-agent",
				TeamName:    "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should update an existing agent_card on conflict", func() {
			data := &agentcard.AgentCardData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "card-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/agent/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-agent",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Version = "2.0.0"
			data.Name = "weather-agent-v2"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			count, err := client.AgentCard.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			card, err := client.AgentCard.Query().
				Where(entagentcard.BasePathEQ("/agent/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(card.Version).To(Equal("2.0.0"))
			Expect(card.Name).To(Equal("weather-agent-v2"))
		})

		It("should set the active-agent-card cache entry when active", func() {
			data := &agentcard.AgentCardData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "card-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/agent/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-agent",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			resolver := infrastructure.NewIDResolver(client, cache)
			id, err := resolver.FindActiveAgentCardID(ctx, "/agent/weather/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))
		})

		It("should clear the active-agent-card cache entry when inactive", func() {
			data := &agentcard.AgentCardData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "card-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/agent/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-agent",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Active = false
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err := resolver.FindActiveAgentCardID(ctx, "/agent/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should be idempotent when the entity does not exist", func() {
			key := agentcard.AgentCardKey{BasePath: "/agent/missing", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should delete an existing agent_card by base path and team name", func() {
			data := &agentcard.AgentCardData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "card-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/agent/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-agent",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := agentcard.AgentCardKey{BasePath: "/agent/weather/v1", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.AgentCard.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err = resolver.FindActiveAgentCardID(ctx, "/agent/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})
	})
})
