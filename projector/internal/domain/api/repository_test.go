// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entapi "github.com/telekom/controlplane/controlplane-api/ent/api"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/domain/api"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockApiDeps implements api.ApiDeps for testing.
type mockApiDeps struct {
	teamIDs map[string]int // key: team name
	teamErr error          // if non-nil, FindTeamID always returns this error
}

func (m *mockApiDeps) FindTeamID(_ context.Context, name string) (int, error) {
	if m.teamErr != nil {
		return 0, m.teamErr
	}
	if id, ok := m.teamIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("team %q: %w", name, infrastructure.ErrEntityNotFound)
}

var _ = Describe("Api Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockApiDeps
		repo   *api.Repository
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

		deps = &mockApiDeps{
			teamIDs: map[string]int{"platform--narvi": teamID},
		}
		repo = api.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an api with valid deps", func() {
			data := &api.ApiData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				BasePath:      "/api/weather/v1",
				Version:       "1.0.0",
				Category:      "g-api",
				Oauth2Scopes:  []string{"scope-a"},
				Active:        true,
				TeamName:      "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			a, err := client.Api.Query().
				Where(entapi.BasePathEQ("/api/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.BasePath).To(Equal("/api/weather/v1"))
			Expect(a.Version).To(Equal("1.0.0"))
			Expect(a.Category).To(Equal("g-api"))
			Expect(a.Oauth2Scopes).To(Equal([]string{"scope-a"}))
			Expect(a.Active).To(BeTrue())

			owner, err := a.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(teamID))
		})

		It("should return ErrDependencyMissing when the owner Team is missing", func() {
			deps.teamIDs = map[string]int{}
			data := &api.ApiData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/api/weather/v1",
				Version:     "1.0.0",
				TeamName:    "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should update an existing api on conflict", func() {
			data := &api.ApiData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/api/weather/v1",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Version = "2.0.0"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			count, err := client.Api.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			a, err := client.Api.Query().
				Where(entapi.BasePathEQ("/api/weather/v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.Version).To(Equal("2.0.0"))
		})

		It("should set the active-api cache entry when active", func() {
			data := &api.ApiData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/api/weather/v1",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			resolver := infrastructure.NewIDResolver(client, cache)
			id, err := resolver.FindActiveApiID(ctx, "/api/weather/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))
		})

		It("should clear the active-api cache entry when inactive", func() {
			data := &api.ApiData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/api/weather/v1",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Active = false
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err := resolver.FindActiveApiID(ctx, "/api/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})

		It("should back-link orphaned ApiExposures projected before the api", func() {
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

			// ApiExposure created first, before its Api exists → stored with a
			// NULL api FK (the create-order race).
			exp, err := client.ApiExposure.Create().
				SetBasePath("/api/weather/v1").
				SetNamespace("prod--platform--narvi").
				SetActive(true).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = exp.QueryAPI().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			// Api appears later → should adopt the orphaned exposure.
			data := &api.ApiData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/api/weather/v1",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			linked, err := exp.QueryAPI().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linked.BasePath).To(Equal("/api/weather/v1"))
		})

		It("should not back-link ApiExposures when the Api is inactive", func() {
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

			exp, err := client.ApiExposure.Create().
				SetBasePath("/api/weather/v1").
				SetNamespace("prod--platform--narvi").
				SetActive(true).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &api.ApiData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/api/weather/v1",
				Version:     "1.0.0",
				Active:      false,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			_, err = exp.QueryAPI().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should be idempotent when the entity does not exist", func() {
			key := api.ApiKey{BasePath: "/api/missing", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should delete an existing api by base path and team name", func() {
			data := &api.ApiData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "api-weather-v1", nil),
				StatusPhase: "READY",
				BasePath:    "/api/weather/v1",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := api.ApiKey{BasePath: "/api/weather/v1", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.Api.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err = resolver.FindActiveApiID(ctx, "/api/weather/v1")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})
	})
})
