// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entagenticexposure "github.com/telekom/controlplane/controlplane-api/ent/agenticexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	"github.com/telekom/controlplane/projector/internal/domain/agenticexposure"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockExposureDeps implements agenticexposure.AgenticExposureDeps for testing.
type mockExposureDeps struct {
	appIDs       map[string]int // key: "appName:teamName"
	appErr       error          // if non-nil, FindApplicationID always returns this error
	mcpServerIDs map[string]int // key: basePath (active mcp_server lookup)
	agentCardIDs map[string]int // key: basePath (active agent_card lookup)
}

func (m *mockExposureDeps) FindApplicationID(_ context.Context, name, teamName string) (int, error) {
	if m.appErr != nil {
		return 0, m.appErr
	}
	key := name + ":" + teamName
	if id, ok := m.appIDs[key]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("application %q (team %q): %w", name, teamName, infrastructure.ErrEntityNotFound)
}

func (m *mockExposureDeps) FindActiveMcpServerID(_ context.Context, basePath string) (int, error) {
	if m.mcpServerIDs != nil {
		if id, ok := m.mcpServerIDs[basePath]; ok {
			return id, nil
		}
	}
	return 0, fmt.Errorf("active mcp_server %q: %w", basePath, infrastructure.ErrEntityNotFound)
}

func (m *mockExposureDeps) FindActiveAgentCardID(_ context.Context, basePath string) (int, error) {
	if m.agentCardIDs != nil {
		if id, ok := m.agentCardIDs[basePath]; ok {
			return id, nil
		}
	}
	return 0, fmt.Errorf("active agent_card %q: %w", basePath, infrastructure.ErrEntityNotFound)
}

var _ = Describe("AgenticExposure Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockExposureDeps
		repo   *agenticexposure.Repository
		ctx    context.Context
		appID  int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		// Seed Zone → Team → Application — required dependency chain.
		z, err := client.Zone.Create().
			SetName("caas").
			SetVisibility(zone.VisibilityEnterprise).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		tm, err := client.Team.Create().
			SetName("platform--narvi").
			SetEmail("narvi@example.com").
			SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		app, err := client.Application.Create().
			SetName("my-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(tm.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appID = app.ID

		deps = &mockExposureDeps{
			appIDs: map[string]int{"my-app:platform--narvi": appID},
		}
		repo = agenticexposure.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an agentic exposure with valid deps", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "exp-1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/mcp/v1/tools",
				Visibility:    "WORLD",
				Variant:       "MCP",
				Active:        true,
				Upstreams:     []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{
					Strategy:     "AUTO",
					TrustedTeams: []string{"team-a"},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/tools")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.BasePath).To(Equal("/mcp/v1/tools"))
			Expect(exp.Visibility.String()).To(Equal("WORLD"))
			Expect(exp.Variant.String()).To(Equal("MCP"))
			Expect(exp.Active).ToNot(BeNil())
			Expect(*exp.Active).To(BeTrue())
			Expect(exp.Upstreams).To(HaveLen(1))
			Expect(exp.Upstreams[0].URL).To(Equal("https://backend.example.com"))
			Expect(exp.ApprovalConfig.Strategy).To(Equal("AUTO"))
			Expect(exp.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-a"}))

			// Verify FK edge.
			owner, err := exp.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(appID))
		})

		It("should link the active McpServer catalogue entry for MCP variant", func() {
			mcp, err := client.McpServer.Create().
				SetBasePath("/mcp/v1/tools").
				SetNamespace("platform--narvi").
				SetVersion("1.0.0").
				SetName("tools-server").
				SetActive(true).
				SetOwnerID(mustCreateTeam(ctx, client, "mcp-owner")).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			deps.mcpServerIDs = map[string]int{"/mcp/v1/tools": mcp.ID}

			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-mcp", nil),
				StatusPhase:    "READY",
				BasePath:       "/mcp/v1/tools",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/tools")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			linkedMcp, err := exp.QueryMcpServer().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linkedMcp.ID).To(Equal(mcp.ID))

			_, err = exp.QueryAgentCard().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should link the active AgentCard catalogue entry for AGENT variant", func() {
			card, err := client.AgentCard.Create().
				SetBasePath("/agent/v1/card").
				SetNamespace("platform--narvi").
				SetVersion("1.0.0").
				SetName("card-agent").
				SetActive(true).
				SetOwnerID(mustCreateTeam(ctx, client, "agent-owner")).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			deps.agentCardIDs = map[string]int{"/agent/v1/card": card.ID}

			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-agent", nil),
				StatusPhase:    "READY",
				BasePath:       "/agent/v1/card",
				Visibility:     "WORLD",
				Variant:        "AGENT",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/agent/v1/card")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			linkedCard, err := exp.QueryAgentCard().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linkedCard.ID).To(Equal(card.ID))

			_, err = exp.QueryMcpServer().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should leave McpServer FK nil when no active catalogue entry exists", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-no-mcp", nil),
				StatusPhase:    "READY",
				BasePath:       "/mcp/v1/missing",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/missing")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			_, err = exp.QueryMcpServer().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should leave AgentCard FK nil when no active catalogue entry exists", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-no-agent", nil),
				StatusPhase:    "READY",
				BasePath:       "/agent/v1/missing",
				Visibility:     "WORLD",
				Variant:        "AGENT",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/agent/v1/missing")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			_, err = exp.QueryAgentCard().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should not resolve catalogue FK when the exposure is inactive", func() {
			mcp, err := client.McpServer.Create().
				SetBasePath("/mcp/v1/inactive-exp").
				SetNamespace("platform--narvi").
				SetVersion("1.0.0").
				SetName("tools-server").
				SetActive(true).
				SetOwnerID(mustCreateTeam(ctx, client, "mcp-owner-2")).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			deps.mcpServerIDs = map[string]int{"/mcp/v1/inactive-exp": mcp.ID}

			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-inactive", nil),
				StatusPhase:    "UNKNOWN",
				BasePath:       "/mcp/v1/inactive-exp",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         false,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/inactive-exp")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			_, err = exp.QueryMcpServer().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should clear the McpServer FK when a variant changes from MCP to AGENT", func() {
			mcp, err := client.McpServer.Create().
				SetBasePath("/mcp/v1/switch").
				SetNamespace("platform--narvi").
				SetVersion("1.0.0").
				SetName("tools-server").
				SetActive(true).
				SetOwnerID(mustCreateTeam(ctx, client, "mcp-owner-3")).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			card, err := client.AgentCard.Create().
				SetBasePath("/mcp/v1/switch").
				SetNamespace("platform--narvi").
				SetVersion("1.0.0").
				SetName("card-agent").
				SetActive(true).
				SetOwnerID(mustCreateTeam(ctx, client, "agent-owner-3")).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			deps.mcpServerIDs = map[string]int{"/mcp/v1/switch": mcp.ID}
			deps.agentCardIDs = map[string]int{"/mcp/v1/switch": card.ID}

			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-switch", nil),
				StatusPhase:    "READY",
				BasePath:       "/mcp/v1/switch",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/switch")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			linkedMcp, err := exp.QueryMcpServer().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linkedMcp.ID).To(Equal(mcp.ID))

			// Now switch the variant to AGENT — the McpServer FK must be
			// cleared and replaced with the AgentCard FK, not left dangling.
			data.Variant = "AGENT"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err = client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/switch")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			linkedCard, err := exp.QueryAgentCard().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linkedCard.ID).To(Equal(card.ID))

			_, err = exp.QueryMcpServer().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should clear the catalogue FK when an active exposure becomes inactive", func() {
			mcp, err := client.McpServer.Create().
				SetBasePath("/mcp/v1/deactivate").
				SetNamespace("platform--narvi").
				SetVersion("1.0.0").
				SetName("tools-server").
				SetActive(true).
				SetOwnerID(mustCreateTeam(ctx, client, "mcp-owner-4")).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			deps.mcpServerIDs = map[string]int{"/mcp/v1/deactivate": mcp.ID}

			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-deactivate", nil),
				StatusPhase:    "READY",
				BasePath:       "/mcp/v1/deactivate",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/deactivate")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = exp.QueryMcpServer().Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Now the exposure becomes inactive — the stale McpServer FK
			// must be cleared, not left pointing at a now-irrelevant entry.
			data.Active = false
			data.StatusPhase = "UNKNOWN"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err = client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/deactivate")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			_, err = exp.QueryMcpServer().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should back-link orphaned subscriptions projected before the exposure", func() {
			// Subscription created first, before its target exposure exists →
			// stored with a NULL target FK (the create-order race).
			sub, err := client.AgenticSubscription.Create().
				SetBasePath("/mcp/v1/orphan").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("orphan-sub").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			// Exposure appears later → should adopt the orphaned subscription.
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "orphan-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				BasePath:       "/mcp/v1/orphan",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         true,
				Upstreams:      []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			target, err := sub.QueryTarget().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(target.BasePath).To(Equal("/mcp/v1/orphan"))
		})

		It("should not back-link subscriptions when the exposure is inactive", func() {
			sub, err := client.AgenticSubscription.Create().
				SetBasePath("/mcp/v1/inactive").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("inactive-sub").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "inactive-exp", nil),
				StatusPhase:    "UNKNOWN",
				BasePath:       "/mcp/v1/inactive",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         false,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should persist security, traffic, and transformation fields", func() {
			clientSecret := "ext-client-secret"
			clientKey := "ext-client-key"
			tokenRequest := "client_secret_basic"
			grantType := "client_credentials"
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "sec-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				BasePath:       "/mcp/v1/secure",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         true,
				Upstreams:      []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
				Security: &model.AgenticExposureSecurity{
					M2M: &model.Machine2MachineAuthentication{
						ExternalIDP: &model.ExternalIdentityProvider{
							TokenEndpoint: "https://idp.example.com/token",
							TokenRequest:  &tokenRequest,
							GrantType:     &grantType,
							Basic: &model.BasicAuthCredentials{
								Username: "ext-user",
								Password: "ext-pass",
							},
							Client: &model.OAuth2ClientCredentials{
								ClientId:     "ext-client-id",
								ClientSecret: &clientSecret,
								ClientKey:    &clientKey,
							},
						},
						Basic: &model.BasicAuthCredentials{
							Username: "svc-user",
							Password: "svc-pass",
						},
						Scopes: []string{"read", "write"},
					},
				},
				Traffic: &model.Traffic{
					RateLimit: &model.RateLimit{
						Provider: &model.RateLimitConfig{
							Limits: model.Limits{
								Second: 10,
								Minute: 100,
								Hour:   1000,
							},
							Options: model.RateLimitOptions{
								HideClientHeaders: true,
								FaultTolerant:     true,
							},
						},
						SubscriberRateLimit: &model.SubscriberRateLimits{
							Default: &model.SubscriberRateLimitDefaults{
								Limits: model.Limits{
									Second: 5,
									Minute: 50,
									Hour:   500,
								},
							},
							Overrides: []model.RateLimitOverrides{
								{
									Subscriber: "sub-a",
									Limits:     model.Limits{Second: 20, Minute: 200, Hour: 2000},
								},
							},
						},
					},
					Failover: &model.Failover{
						Zones: []string{"zoneA", "zoneB"},
					},
				},
				Transformation: &model.AgenticTransformation{
					Request: model.AgenticRequestResponseTransformation{
						Headers: model.AgenticHeaderTransformation{
							Remove: []string{"X-Remove-Me"},
							Add:    []string{"X-Add-Me: value"},
						},
					},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/secure")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Security assertions
			Expect(exp.Security.M2M).NotTo(BeNil())
			Expect(exp.Security.M2M.Basic).NotTo(BeNil())
			Expect(exp.Security.M2M.Basic.Username).To(Equal("svc-user"))
			Expect(exp.Security.M2M.Basic.Password).To(Equal("svc-pass"))
			Expect(exp.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
			Expect(exp.Security.M2M.ExternalIDP).NotTo(BeNil())
			Expect(exp.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
			Expect(*exp.Security.M2M.ExternalIDP.TokenRequest).To(Equal("client_secret_basic"))
			Expect(*exp.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
			Expect(exp.Security.M2M.ExternalIDP.Basic.Username).To(Equal("ext-user"))
			Expect(exp.Security.M2M.ExternalIDP.Basic.Password).To(Equal("ext-pass"))
			Expect(exp.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("ext-client-id"))
			Expect(*exp.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("ext-client-secret"))
			Expect(*exp.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal("ext-client-key"))

			// RateLimit assertions
			Expect(exp.Traffic.RateLimit.Provider).NotTo(BeNil())
			Expect(exp.Traffic.RateLimit.Provider.Limits.Second).To(Equal(10))
			Expect(exp.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(100))
			Expect(exp.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(1000))
			Expect(exp.Traffic.RateLimit.Provider.Options.HideClientHeaders).To(BeTrue())
			Expect(exp.Traffic.RateLimit.Provider.Options.FaultTolerant).To(BeTrue())
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit).NotTo(BeNil())
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Second).To(Equal(5))
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Minute).To(Equal(50))
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Hour).To(Equal(500))
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Overrides).To(HaveLen(1))
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Subscriber).To(Equal("sub-a"))
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Second).To(Equal(20))
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Minute).To(Equal(200))
			Expect(exp.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Hour).To(Equal(2000))

			// Failover
			Expect(exp.Traffic.Failover.Zones).To(Equal([]string{"zoneA", "zoneB"}))

			// Transformation
			Expect(exp.Transformation.Request.Headers.Remove).To(Equal([]string{"X-Remove-Me"}))
			Expect(exp.Transformation.Request.Headers.Add).To(Equal([]string{"X-Add-Me: value"}))
		})

		It("should return ErrDependencyMissing when application is missing", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "fail-exp", nil),
				StatusPhase:    "UNKNOWN",
				StatusMessage:  "",
				BasePath:       "/mcp/v1/fail",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "missing-app",
				TeamName:       "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("application"))
		})

		It("should propagate non-ErrEntityNotFound errors from FindApplicationID", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockExposureDeps{
				appIDs: map[string]int{},
				appErr: dbErr,
			}
			failRepo := agenticexposure.NewRepository(client, cache, failDeps)

			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "fail-exp", nil),
				StatusPhase:    "UNKNOWN",
				StatusMessage:  "",
				BasePath:       "/mcp/v1/fail",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should update existing exposure on conflict with UpdateNewValues", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "upd-exp", nil),
				StatusPhase:    "PENDING",
				StatusMessage:  "v1",
				BasePath:       "/mcp/v1/update",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      []model.Upstream{{URL: "https://old.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update with new values.
			data.StatusPhase = "READY"
			data.StatusMessage = "v2"
			data.Visibility = "WORLD"
			data.Active = true
			data.Upstreams = []model.Upstream{{URL: "https://new.example.com", Weight: 50}}
			data.ApprovalConfig = model.ApprovalConfig{Strategy: "FOUR_EYES", TrustedTeams: []string{"t1"}}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/update")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.StatusPhase).ToNot(BeNil())
			Expect(exp.StatusPhase.String()).To(Equal("READY"))
			Expect(exp.StatusMessage).ToNot(BeNil())
			Expect(*exp.StatusMessage).To(Equal("v2"))
			Expect(exp.Visibility.String()).To(Equal("WORLD"))
			Expect(exp.Active).ToNot(BeNil())
			Expect(*exp.Active).To(BeTrue())
			Expect(exp.Upstreams[0].URL).To(Equal("https://new.example.com"))
			Expect(exp.ApprovalConfig.Strategy).To(Equal("FOUR_EYES"))
		})

		It("should update variant, security and transformation on upsert conflict", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "var-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				BasePath:       "/mcp/v1/variant",
				Visibility:     "WORLD",
				Variant:        "MCP",
				Active:         true,
				Upstreams:      []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/variant")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.Variant.String()).To(Equal("MCP"))
			Expect(exp.Security.M2M).To(BeNil())
			Expect(exp.Transformation.Request.Headers.Remove).To(BeEmpty())

			// Update with a different variant, security, and transformation.
			data.Variant = "TELECONTEXTMCP"
			data.Security = &model.AgenticExposureSecurity{
				M2M: &model.Machine2MachineAuthentication{
					Scopes: []string{"read", "write"},
				},
			}
			data.Transformation = &model.AgenticTransformation{
				Request: model.AgenticRequestResponseTransformation{
					Headers: model.AgenticHeaderTransformation{
						Add: []string{"X-Trace-Id: abc"},
					},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err = client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/variant")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.Variant.String()).To(Equal("TELECONTEXTMCP"))
			Expect(exp.Security.M2M).NotTo(BeNil())
			Expect(exp.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
			Expect(exp.Transformation.Request.Headers.Add).To(Equal([]string{"X-Trace-Id: abc"}))
		})

		It("should populate the edge cache after upsert", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "cached-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/mcp/v1/cached",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("agenticexposure", "/mcp/v1/cached:my-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing agentic exposure", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "del-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/mcp/v1/delete",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := agenticexposure.AgenticExposureKey{
				BasePath: "/mcp/v1/delete",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.AgenticExposure.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should be idempotent for non-existent exposure", func() {
			key := agenticexposure.AgenticExposureKey{
				BasePath: "/mcp/v1/nonexistent",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "evict-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/mcp/v1/evict",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("agenticexposure", "/mcp/v1/evict:my-app:platform--narvi")
			Expect(found).To(BeTrue())

			key := agenticexposure.AgenticExposureKey{
				BasePath: "/mcp/v1/evict",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()

			_, found = cache.Get("agenticexposure", "/mcp/v1/evict:my-app:platform--narvi")
			Expect(found).To(BeFalse())
		})

		It("should only delete the targeted exposure", func() {
			data1 := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-1", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/mcp/v1/first",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			data2 := &agenticexposure.AgenticExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-2", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/mcp/v1/second",
				Visibility:     "ENTERPRISE",
				Variant:        "MCP",
				Active:         false,
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data1)).To(Succeed())
			Expect(repo.Upsert(ctx, data2)).To(Succeed())

			key := agenticexposure.AgenticExposureKey{
				BasePath: "/mcp/v1/first",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Second exposure should still exist.
			count, err := client.AgenticExposure.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			exp, err := client.AgenticExposure.Query().
				Where(entagenticexposure.BasePathEQ("/mcp/v1/second")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.BasePath).To(Equal("/mcp/v1/second"))
		})
	})

	// ── IDResolver FindAgenticExposureID ───────────────────────────────

	Describe("IDResolver FindAgenticExposureID", func() {
		var resolver *infrastructure.IDResolver

		BeforeEach(func() {
			resolver = infrastructure.NewIDResolver(client, cache)
		})

		It("should return cached ID on cache hit", func() {
			cache.Set("agenticexposure", "/mcp/v1/hit:my-app:platform--narvi", 42)
			cache.Wait()

			id, err := resolver.FindAgenticExposureID(ctx, "/mcp/v1/hit", "my-app", "platform--narvi")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(42))
		})

		It("should fall back to DB on cache miss and cache the result", func() {
			exp, err := client.AgenticExposure.Create().
				SetBasePath("/mcp/v1/db-lookup").
				SetNamespace("platform--narvi").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			id, err := resolver.FindAgenticExposureID(ctx, "/mcp/v1/db-lookup", "my-app", "platform--narvi")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(exp.ID))

			cache.Wait()
			cachedID, found := cache.Get("agenticexposure", "/mcp/v1/db-lookup:my-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(cachedID).To(Equal(exp.ID))
		})

		It("should return ErrEntityNotFound for missing exposure", func() {
			_, err := resolver.FindAgenticExposureID(ctx, "/mcp/v1/missing", "my-app", "platform--narvi")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("/mcp/v1/missing"))
		})
	})
})

// mustCreateTeam creates a Team row with the given name and returns its ID.
// Used to satisfy McpServer/AgentCard's required owner Team FK in tests that
// don't otherwise need the team beyond ownership.
func mustCreateTeam(ctx context.Context, client *ent.Client, name string) int {
	tm, err := client.Team.Create().
		SetName(name).
		SetEmail(name + "@example.com").
		SetNamespace(name).
		Save(ctx)
	Expect(err).NotTo(HaveOccurred())
	return tm.ID
}
