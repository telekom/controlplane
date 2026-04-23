// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entapp "github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/domain/application"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockAppDeps implements application.ApplicationDeps for testing.
type mockAppDeps struct {
	teamIDs map[string]int
	zoneIDs map[string]int
	teamErr error // if non-nil, FindTeamID always returns this error
	zoneErr error // if non-nil, FindZoneID always returns this error
}

func (m *mockAppDeps) FindTeamID(_ context.Context, name string) (int, error) {
	if m.teamErr != nil {
		return 0, m.teamErr
	}
	if id, ok := m.teamIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("team %q: %w", name, infrastructure.ErrEntityNotFound)
}

func (m *mockAppDeps) FindZoneID(_ context.Context, name string) (int, error) {
	if m.zoneErr != nil {
		return 0, m.zoneErr
	}
	if id, ok := m.zoneIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("zone %q: %w", name, infrastructure.ErrEntityNotFound)
}

var _ = Describe("Application Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockAppDeps
		repo   *application.Repository
		ctx    context.Context
		teamID int
		zoneID int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		// Seed Zone and Team — required dependencies for Application.
		z, err := client.Zone.Create().
			SetName("caas").
			SetVisibility(zone.VisibilityEnterprise).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		zoneID = z.ID

		t, err := client.Team.Create().
			SetName("platform--narvi").
			SetEmail("narvi@example.com").
			SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		teamID = t.ID

		deps = &mockAppDeps{
			teamIDs: map[string]int{"platform--narvi": teamID},
			zoneIDs: map[string]int{"caas": zoneID},
		}
		repo = application.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an application with valid deps", func() {
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "my-app", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				Name:          "my-app",
				ClientID:      strPtr("client-123"),
				IssuerURL:     nil,
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			app, err := client.Application.Query().Where(entapp.NameEQ("my-app")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(app.Name).To(Equal("my-app"))
			Expect(app.ClientID).ToNot(BeNil())
			Expect(*app.ClientID).To(Equal("client-123"))
			Expect(app.IssuerURL).To(BeNil())

			// Verify FK edges.
			ownerTeam, err := app.QueryOwnerTeam().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ownerTeam.ID).To(Equal(teamID))

			appZone, err := app.QueryZone().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(appZone.ID).To(Equal(zoneID))
		})

		It("should return ErrDependencyMissing when team is missing", func() {
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--unknown--team-a", "fail-app", nil),
				StatusPhase:   "UNKNOWN",
				StatusMessage: "",
				Name:          "fail-app",
				TeamName:      "unknown--team-a",
				ZoneName:      "caas",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("team"))
		})

		It("should return ErrDependencyMissing when zone is missing", func() {
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "fail-app", nil),
				StatusPhase:   "UNKNOWN",
				StatusMessage: "",
				Name:          "fail-app",
				TeamName:      "platform--narvi",
				ZoneName:      "missing-zone",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("zone"))
		})

		It("should propagate non-ErrEntityNotFound errors from FindTeamID", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockAppDeps{
				teamIDs: map[string]int{},
				zoneIDs: map[string]int{"caas": zoneID},
				teamErr: dbErr,
			}
			failRepo := application.NewRepository(client, cache, failDeps)

			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "fail-app", nil),
				StatusPhase:   "UNKNOWN",
				StatusMessage: "",
				Name:          "fail-app",
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should propagate non-ErrEntityNotFound errors from FindZoneID", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockAppDeps{
				teamIDs: map[string]int{"platform--narvi": teamID},
				zoneIDs: map[string]int{},
				zoneErr: dbErr,
			}
			failRepo := application.NewRepository(client, cache, failDeps)

			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "fail-app", nil),
				StatusPhase:   "UNKNOWN",
				StatusMessage: "",
				Name:          "fail-app",
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should update existing application on conflict", func() {
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "upd-app", nil),
				StatusPhase:   "PENDING",
				StatusMessage: "v1",
				Name:          "upd-app",
				ClientID:      nil,
				IssuerURL:     nil,
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update with new values.
			data.StatusPhase = "READY"
			data.StatusMessage = "v2"
			data.ClientID = strPtr("client-456")
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			app, err := client.Application.Query().Where(entapp.NameEQ("upd-app")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(app.StatusPhase).ToNot(BeNil())
			Expect(app.StatusPhase.String()).To(Equal("READY"))
			Expect(app.StatusMessage).ToNot(BeNil())
			Expect(*app.StatusMessage).To(Equal("v2"))
			Expect(app.ClientID).ToNot(BeNil())
			Expect(*app.ClientID).To(Equal("client-456"))
		})

		It("should clear IssuerURL when updated to nil", func() {
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "issuer-app", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				Name:          "issuer-app",
				IssuerURL:     strPtr("https://issuer.example.com"),
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify IssuerURL was set.
			app, err := client.Application.Query().Where(entapp.NameEQ("issuer-app")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(app.IssuerURL).ToNot(BeNil())
			Expect(*app.IssuerURL).To(Equal("https://issuer.example.com"))

			// Update with IssuerURL cleared.
			data.IssuerURL = nil
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			app, err = client.Application.Query().Where(entapp.NameEQ("issuer-app")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(app.IssuerURL).To(BeNil())
		})

		It("should populate the edge cache after upsert", func() {
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "cached-app", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "cached-app",
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("application", "cached-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete application and cascade to children", func() {
			// Create an application first.
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "del-app", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "del-app",
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Create child ApiExposure to verify cascade.
			app, err := client.Application.Query().Where(entapp.NameEQ("del-app")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.ApiExposure.Create().
				SetBasePath("/api/v1").
				SetNamespace("platform--narvi").
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			key := application.ApplicationKey{Name: "del-app", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Verify application is gone.
			count, err := client.Application.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			// Verify child ApiExposure was cascade-deleted.
			expCount, err := client.ApiExposure.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(expCount).To(Equal(0))
		})

		It("should be idempotent for non-existent application", func() {
			key := application.ApplicationKey{Name: "nonexistent", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "evict-app", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "evict-app",
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("application", "evict-app:platform--narvi")
			Expect(found).To(BeTrue())

			key := application.ApplicationKey{Name: "evict-app", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()

			_, found = cache.Get("application", "evict-app:platform--narvi")
			Expect(found).To(BeFalse())
		})

		It("should only delete the targeted application", func() {
			data1 := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "app-1", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "app-1",
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			data2 := &application.ApplicationData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "app-2", nil),
				StatusPhase:   "READY",
				StatusMessage: "",
				Name:          "app-2",
				TeamName:      "platform--narvi",
				ZoneName:      "caas",
			}
			Expect(repo.Upsert(ctx, data1)).To(Succeed())
			Expect(repo.Upsert(ctx, data2)).To(Succeed())

			key := application.ApplicationKey{Name: "app-1", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// app-2 should still exist.
			count, err := client.Application.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			app2, err := client.Application.Query().Where(entapp.NameEQ("app-2")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(app2.Name).To(Equal("app-2"))
		})
	})

	// ── IDResolver FindApplicationID ───────────────────────────────────

	Describe("IDResolver FindApplicationID", func() {
		var resolver *infrastructure.IDResolver

		BeforeEach(func() {
			resolver = infrastructure.NewIDResolver(client, cache)
		})

		It("should return cached ID on cache hit", func() {
			cache.Set("application", "hit-app:platform--narvi", 88)
			cache.Wait()

			id, err := resolver.FindApplicationID(ctx, "hit-app", "platform--narvi")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(88))
		})

		It("should fall back to DB on cache miss and cache the result", func() {
			// Create app directly in DB.
			app, err := client.Application.Create().
				SetName("db-app").
				SetNamespace("platform--narvi").
				SetOwnerTeamID(teamID).
				SetZoneID(zoneID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			id, err := resolver.FindApplicationID(ctx, "db-app", "platform--narvi")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal(app.ID))

			cache.Wait()
			cachedID, found := cache.Get("application", "db-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(cachedID).To(Equal(app.ID))
		})

		It("should return ErrEntityNotFound for missing application", func() {
			_, err := resolver.FindApplicationID(ctx, "missing-app", "platform--narvi")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("missing-app"))
		})
	})
})

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
