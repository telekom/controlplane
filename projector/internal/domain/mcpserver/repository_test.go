// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entagenticexposure "github.com/telekom/controlplane/controlplane-api/ent/agenticexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	entmcpserver "github.com/telekom/controlplane/controlplane-api/ent/mcpserver"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/domain/mcpserver"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockMCPServerDeps implements mcpserver.MCPServerDeps for testing.
type mockMCPServerDeps struct {
	teamIDs map[string]int // key: team name
	teamErr error          // if non-nil, FindTeamID always returns this error
}

func (m *mockMCPServerDeps) FindTeamID(_ context.Context, name string) (int, error) {
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
		deps   *mockMCPServerDeps
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

		deps = &mockMCPServerDeps{
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
			data := &mcpserver.MCPServerData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/mcp/weather/v1",
				Version:       "1.0.0",
				Name:          "weather-server",
				Description:   "Weather MCP server",
				Category:      "g-api",
				OAuth2Scopes:  []string{"scope-a"},
				Active:        true,
				TeamName:      "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			mcp, err := client.MCPServer.Query().
				Where(entmcpserver.BasePathEQ("/mcp/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcp.BasePath).To(Equal("/mcp/weather/v1"))
			Expect(mcp.Version).To(Equal("1.0.0"))
			Expect(mcp.Name).To(Equal("weather-server"))
			Expect(mcp.Description).To(Equal("Weather MCP server"))
			Expect(mcp.Category).To(Equal("g-api"))
			Expect(mcp.OAuth2Scopes).To(Equal([]string{"scope-a"}))
			Expect(mcp.Active).To(BeTrue())

			owner, err := mcp.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(teamID))
		})

		It("should return ErrDependencyMissing when the owner Team is missing", func() {
			deps.teamIDs = map[string]int{}
			data := &mcpserver.MCPServerData{
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
			data := &mcpserver.MCPServerData{
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

			count, err := client.MCPServer.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			mcp, err := client.MCPServer.Query().
				Where(entmcpserver.BasePathEQ("/mcp/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcp.Version).To(Equal("2.0.0"))
			Expect(mcp.Name).To(Equal("weather-server-v2"))
		})

		It("should set the active-mcp-server cache entry when active", func() {
			data := &mcpserver.MCPServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			resolver := infrastructure.NewIDResolver(client, cache)
			id, err := resolver.FindActiveMCPServerID(ctx, "/mcp/weather/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))
		})

		It("should clear the active-mcp-server cache entry when inactive", func() {
			data := &mcpserver.MCPServerData{
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
			cache.Wait()

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err := resolver.FindActiveMCPServerID(ctx, "/mcp/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})

		It("should back-link orphaned AgenticExposures projected before the mcp_server", func() {
			// Seed an Application (owner) via Zone → Team → Application.
			z, err := client.Zone.Create().
				SetName("caas").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			app, err := client.Application.Create().
				SetName("my-app").
				SetNamespace("platform--narvi").
				SetOwnerTeamID(teamID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			// AgenticExposure created first, before its McpServer exists → stored
			// with a NULL mcp_server FK (the create-order race).
			exp, err := client.AgenticExposure.Create().
				SetBasePath("/mcp/weather/v1").
				SetNamespace("prod--platform--narvi").
				SetVariant(entagenticexposure.VariantMCP).
				SetActive(true).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = exp.QueryMCPServer().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			// McpServer appears later → should adopt the orphaned exposure.
			data := &mcpserver.MCPServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			srv, err := exp.QueryMCPServer().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(srv.BasePath).To(Equal("/mcp/weather/v1"))
		})

		It("should not back-link AgenticExposures with a different variant", func() {
			z, err := client.Zone.Create().
				SetName("caas").
				SetVisibility(zone.VisibilityEnterprise).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			app, err := client.Application.Create().
				SetName("my-app").
				SetNamespace("platform--narvi").
				SetOwnerTeamID(teamID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			exp, err := client.AgenticExposure.Create().
				SetBasePath("/mcp/weather/v1").
				SetNamespace("prod--platform--narvi").
				SetVariant(entagenticexposure.VariantAgent).
				SetActive(true).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &mcpserver.MCPServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			_, err = exp.QueryMCPServer().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should be idempotent when the entity does not exist", func() {
			key := mcpserver.MCPServerKey{BasePath: "/mcp/missing", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should delete an existing mcp_server by base path and team name", func() {
			data := &mcpserver.MCPServerData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "mcp-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/mcp/weather/v1",
				Version:     "1.0.0",
				Name:        "weather-server",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := mcpserver.MCPServerKey{BasePath: "/mcp/weather/v1", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.MCPServer.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err = resolver.FindActiveMCPServerID(ctx, "/mcp/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})
	})
})
