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
