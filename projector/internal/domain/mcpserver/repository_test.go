// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entmcpserver "github.com/telekom/controlplane/controlplane-api/ent/mcpserver"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	"github.com/telekom/controlplane/projector/internal/domain/mcpserver"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockMcpServerDeps implements mcpserver.McpServerDeps for testing.
type mockMcpServerDeps struct {
	teamIDs map[string]int // key: team name
	teamErr error          // if non-nil, FindTeamID always returns this error
}

func (m *mockMcpServerDeps) FindTeamID(_ context.Context, name string) (int, error) {
	if m.teamErr != nil {
		return 0, m.teamErr
	}
	if id, ok := m.teamIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("team %q: %w", name, infrastructure.ErrEntityNotFound)
}

var _ = Describe("McpServer Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockMcpServerDeps
		repo   *mcpserver.Repository
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

		deps = &mockMcpServerDeps{
			teamIDs: map[string]int{"platform--narvi": teamID},
		}
		repo = mcpserver.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an mcp_server with valid deps", func() {
			data := &mcpserver.McpServerData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/mcp/weather/v1",
				Version:       "1.0.0",
				Name:          "weather-server",
				Description:   "Weather MCP server",
				Category:      "g-api",
				Oauth2Scopes:  []string{"scope-a"},
				Active:        true,
				TeamName:      "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			mcp, err := client.McpServer.Query().
				Where(entmcpserver.BasePathEQ("/mcp/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcp.BasePath).To(Equal("/mcp/weather/v1"))
			Expect(mcp.Version).To(Equal("1.0.0"))
			Expect(mcp.Name).To(Equal("weather-server"))
			Expect(mcp.Description).To(Equal("Weather MCP server"))
			Expect(mcp.Category).To(Equal("g-api"))
			Expect(mcp.Oauth2Scopes).To(Equal([]string{"scope-a"}))
			Expect(mcp.Active).To(BeTrue())

			owner, err := mcp.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(teamID))
		})

		It("should return ErrDependencyMissing when the owner Team is missing", func() {
			deps.teamIDs = map[string]int{}
			data := &mcpserver.McpServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				TeamName:    "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should update an existing mcp_server on conflict", func() {
			data := &mcpserver.McpServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Version = "2.0.0"
			data.Name = "weather-server-v2"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			count, err := client.McpServer.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			mcp, err := client.McpServer.Query().
				Where(entmcpserver.BasePathEQ("/mcp/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcp.Version).To(Equal("2.0.0"))
			Expect(mcp.Name).To(Equal("weather-server-v2"))
		})

		It("should set the active-mcp-server cache entry when active", func() {
			data := &mcpserver.McpServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			resolver := infrastructure.NewIDResolver(client, cache)
			id, err := resolver.FindActiveMcpServerID(ctx, "/mcp/weather/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))
		})

		It("should clear the active-mcp-server cache entry when inactive", func() {
			data := &mcpserver.McpServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Active = false
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err := resolver.FindActiveMcpServerID(ctx, "/mcp/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should be idempotent when the entity does not exist", func() {
			key := mcpserver.McpServerKey{BasePath: "/mcp/missing", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should delete an existing mcp_server by base path and team name", func() {
			data := &mcpserver.McpServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := mcpserver.McpServerKey{BasePath: "/mcp/weather/v1", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.McpServer.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err = resolver.FindActiveMcpServerID(ctx, "/mcp/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})
	})
})
