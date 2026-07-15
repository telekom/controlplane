// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entapiexposure "github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	"github.com/telekom/controlplane/projector/internal/domain/apiexposure"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockExposureDeps implements apiexposure.APIExposureDeps for testing.
type mockExposureDeps struct {
	appIDs map[string]int // key: "appName:teamName"
	appErr error          // if non-nil, FindApplicationID always returns this error
	apiIDs map[string]int // key: basePath (active api lookup)
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

func (m *mockExposureDeps) FindActiveApiID(_ context.Context, basePath string) (int, error) {
	if m.apiIDs != nil {
		if id, ok := m.apiIDs[basePath]; ok {
			return id, nil
		}
	}
	return 0, fmt.Errorf("active api %q: %w", basePath, infrastructure.ErrEntityNotFound)
}

var _ = Describe("ApiExposure Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockExposureDeps
		repo   *apiexposure.Repository
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

		t, err := client.Team.Create().
			SetName("platform--narvi").
			SetEmail("narvi@example.com").
			SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		app, err := client.Application.Create().
			SetName("my-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appID = app.ID

		deps = &mockExposureDeps{
			appIDs: map[string]int{"my-app:platform--narvi": appID},
		}
		repo = apiexposure.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an api exposure with valid deps", func() {
			data := &apiexposure.APIExposureData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "exp-1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/api/v1/users",
				Visibility:    "WORLD",
				Active:        true,
				Features:      []string{},
				Upstreams:     []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{
					Strategy:     "AUTO",
					TrustedTeams: []string{"team-a"},
				},
				APIVersion: nil,
				AppName:    "my-app",
				TeamName:   "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.ApiExposure.Query().
				Where(entapiexposure.BasePathEQ("/api/v1/users")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.BasePath).To(Equal("/api/v1/users"))
			Expect(exp.Visibility.String()).To(Equal("WORLD"))
			Expect(exp.Active).ToNot(BeNil())
			Expect(*exp.Active).To(BeTrue())
			Expect(exp.Features).To(Equal([]string{}))
			Expect(exp.Upstreams).To(HaveLen(1))
			Expect(exp.Upstreams[0].URL).To(Equal("https://backend.example.com"))
			Expect(exp.ApprovalConfig.Strategy).To(Equal("AUTO"))
			Expect(exp.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-a"}))
			Expect(exp.APIVersion).To(BeNil())

			// Verify FK edge.
			owner, err := exp.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(appID))
		})

		It("should back-link orphaned subscriptions projected before the exposure", func() {
			// Subscription created first, before its target exposure exists →
			// stored with a NULL target FK (the create-order race).
			sub, err := client.ApiSubscription.Create().
				SetBasePath("/api/v1/orphan").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("orphan-sub").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			// Exposure appears later → should adopt the orphaned subscription.
			data := &apiexposure.APIExposureData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "orphan-exp", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/api/v1/orphan",
				Visibility:    "WORLD",
				Active:        true,
				Features:      []string{},
				Upstreams:     []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				AppName:       "my-app",
				TeamName:      "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			target, err := sub.QueryTarget().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(target.BasePath).To(Equal("/api/v1/orphan"))
		})

		It("should not back-link subscriptions when the exposure is inactive", func() {
			sub, err := client.ApiSubscription.Create().
				SetBasePath("/api/v1/inactive").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("inactive-sub").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &apiexposure.APIExposureData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "inactive-exp", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/api/v1/inactive",
				Visibility:    "WORLD",
				Active:        false,
				Features:      []string{},
				Upstreams:     []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				AppName:       "my-app",
				TeamName:      "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should persist security and rate_limit fields", func() {
			clientSecret := "ext-client-secret"
			clientKey := "ext-client-key"
			tokenRequest := "client_secret_basic"
			grantType := "client_credentials"
			data := &apiexposure.APIExposureData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "sec-exp", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/api/v1/secure",
				Visibility:    "WORLD",
				Active:        true,
				Features:      []string{},
				Upstreams:     []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{
					Strategy: "AUTO",
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
				Security: &model.ApiExposureSecurity{
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
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.ApiExposure.Query().
				Where(entapiexposure.BasePathEQ("/api/v1/secure")).
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

		})

		It("should return ErrDependencyMissing when application is missing", func() {
			data := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "fail-exp", nil),
				StatusPhase:    "UNKNOWN",
				StatusMessage:  "",
				BasePath:       "/api/v1/fail",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
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
			failRepo := apiexposure.NewRepository(client, cache, failDeps)

			data := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "fail-exp", nil),
				StatusPhase:    "UNKNOWN",
				StatusMessage:  "",
				BasePath:       "/api/v1/fail",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
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
			data := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "upd-exp", nil),
				StatusPhase:    "PENDING",
				StatusMessage:  "v1",
				BasePath:       "/api/v1/update",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
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

			exp, err := client.ApiExposure.Query().
				Where(entapiexposure.BasePathEQ("/api/v1/update")).
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

		It("should update features, security and traffic on upsert conflict", func() {
			data := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "feat-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				BasePath:       "/api/v1/features",
				Visibility:     "WORLD",
				Active:         true,
				Features:       []string{"LAST_MILE_SECURITY"},
				Upstreams:      []model.Upstream{{URL: "https://backend.example.com", Weight: 100}},
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.ApiExposure.Query().
				Where(entapiexposure.BasePathEQ("/api/v1/features")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.Features).To(HaveLen(1))
			Expect(exp.Features).To(ContainElement("LAST_MILE_SECURITY"))
			Expect(exp.Security.M2M).To(BeNil())
			Expect(exp.Traffic.RateLimit).To(BeNil())

			// Update with security, traffic, and expanded features.
			data.Features = []string{"LAST_MILE_SECURITY", "EXTERNAL_IDP", "CUSTOM_SCOPES", "RATE_LIMIT", "FAILOVER"}
			data.Security = &model.ApiExposureSecurity{
				M2M: &model.Machine2MachineAuthentication{
					Scopes: []string{"read", "write"},
				},
			}
			data.Traffic = &model.Traffic{
				RateLimit: &model.RateLimit{
					Provider: &model.RateLimitConfig{
						Limits: model.Limits{Second: 10, Minute: 100, Hour: 1000},
					},
				},
				Failover: &model.Failover{
					Zones: []string{"zone-a"},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err = client.ApiExposure.Query().
				Where(entapiexposure.BasePathEQ("/api/v1/features")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.Features).To(HaveLen(5))
			Expect(exp.Features).To(ContainElements("LAST_MILE_SECURITY", "EXTERNAL_IDP", "CUSTOM_SCOPES", "RATE_LIMIT", "FAILOVER"))
			Expect(exp.Security.M2M).NotTo(BeNil())
			Expect(exp.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
			Expect(exp.Traffic.RateLimit).NotTo(BeNil())
			Expect(exp.Traffic.RateLimit.Provider.Limits.Second).To(Equal(10))
			Expect(exp.Traffic.Failover).NotTo(BeNil())
			Expect(exp.Traffic.Failover.Zones).To(Equal([]string{"zone-a"}))

			// Update again: remove security features, add load balancing.
			data.Features = []string{"LAST_MILE_SECURITY", "RATE_LIMIT", "LOAD_BALANCING"}
			data.Security = nil
			data.Upstreams = []model.Upstream{
				{URL: "https://primary.example.com", Weight: 80},
				{URL: "https://secondary.example.com", Weight: 20},
			}
			data.Traffic = &model.Traffic{
				RateLimit: &model.RateLimit{
					Provider: &model.RateLimitConfig{
						Limits: model.Limits{Second: 50, Minute: 500, Hour: 5000},
					},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err = client.ApiExposure.Query().
				Where(entapiexposure.BasePathEQ("/api/v1/features")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.Features).To(HaveLen(3))
			Expect(exp.Features).To(ContainElements("LAST_MILE_SECURITY", "RATE_LIMIT", "LOAD_BALANCING"))
			Expect(exp.Security.M2M).To(BeNil())
			Expect(exp.Upstreams).To(HaveLen(2))
			Expect(exp.Traffic.RateLimit.Provider.Limits.Second).To(Equal(50))
			Expect(exp.Traffic.Failover).To(BeNil())
		})

		It("should populate the edge cache after upsert", func() {
			data := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "cached-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/api/v1/cached",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("apiexposure", "/api/v1/cached:my-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing api exposure", func() {
			data := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "del-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/api/v1/delete",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := apiexposure.APIExposureKey{
				BasePath: "/api/v1/delete",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.ApiExposure.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should be idempotent for non-existent exposure", func() {
			key := apiexposure.APIExposureKey{
				BasePath: "/api/v1/nonexistent",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "evict-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/api/v1/evict",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("apiexposure", "/api/v1/evict:my-app:platform--narvi")
			Expect(found).To(BeTrue())

			key := apiexposure.APIExposureKey{
				BasePath: "/api/v1/evict",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()

			_, found = cache.Get("apiexposure", "/api/v1/evict:my-app:platform--narvi")
			Expect(found).To(BeFalse())
		})

		It("should only delete the targeted exposure", func() {
			data1 := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-1", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/api/v1/first",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			data2 := &apiexposure.APIExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-2", nil),
				StatusPhase:    "READY",
				StatusMessage:  "",
				BasePath:       "/api/v1/second",
				Visibility:     "ENTERPRISE",
				Active:         false,
				Features:       []string{},
				Upstreams:      nil,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data1)).To(Succeed())
			Expect(repo.Upsert(ctx, data2)).To(Succeed())

			key := apiexposure.APIExposureKey{
				BasePath: "/api/v1/first",
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Second exposure should still exist.
			count, err := client.ApiExposure.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			exp, err := client.ApiExposure.Query().
				Where(entapiexposure.BasePathEQ("/api/v1/second")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.BasePath).To(Equal("/api/v1/second"))
		})
	})

	// ── IDResolver FindAPIExposureID ───────────────────────────────────

	Describe("IDResolver FindAPIExposureID", func() {
		var resolver *infrastructure.IDResolver

		BeforeEach(func() {
			resolver = infrastructure.NewIDResolver(client, cache)
		})

		It("should return cached ID on cache hit", func() {
			cache.Set("apiexposure", "/api/v1/hit:my-app:platform--narvi", 42)
			cache.Wait()

			id, err := resolver.FindAPIExposureID(ctx, "/api/v1/hit", "my-app", "platform--narvi")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(42))
		})

		It("should fall back to DB on cache miss and cache the result", func() {
			exp, err := client.ApiExposure.Create().
				SetBasePath("/api/v1/db-lookup").
				SetNamespace("platform--narvi").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			id, err := resolver.FindAPIExposureID(ctx, "/api/v1/db-lookup", "my-app", "platform--narvi")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(exp.ID))

			cache.Wait()
			cachedID, found := cache.Get("apiexposure", "/api/v1/db-lookup:my-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(cachedID).To(Equal(exp.ID))
		})

		It("should return ErrEntityNotFound for missing exposure", func() {
			_, err := resolver.FindAPIExposureID(ctx, "/api/v1/missing", "my-app", "platform--narvi")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("/api/v1/missing"))
		})
	})
})
