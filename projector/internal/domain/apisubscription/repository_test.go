// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entapiexposure "github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	entapisub "github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/domain/apisubscription"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockSubscriptionDeps implements apisubscription.APISubscriptionDeps for testing.
type mockSubscriptionDeps struct {
	appIDs      map[string]int // key: "appName:teamName"
	exposureIDs map[string]int // key: basePath
	appErr      error          // if non-nil, FindApplicationID always returns this error
}

func (m *mockSubscriptionDeps) FindApplicationID(_ context.Context, name, teamName string) (int, error) {
	if m.appErr != nil {
		return 0, m.appErr
	}
	key := name + ":" + teamName
	if id, ok := m.appIDs[key]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("application %q (team %q): %w", name, teamName, infrastructure.ErrEntityNotFound)
}

func (m *mockSubscriptionDeps) FindAPIExposureByBasePath(_ context.Context, basePath string) (int, error) {
	if id, ok := m.exposureIDs[basePath]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("api_exposure basePath %q: %w", basePath, infrastructure.ErrEntityNotFound)
}

var _ = Describe("ApiSubscription Repository", func() {
	var (
		client     *ent.Client
		cache      *infrastructure.EdgeCache
		deps       *mockSubscriptionDeps
		repo       *apisubscription.Repository
		ctx        context.Context
		appID      int
		exposureID int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		// Seed Zone → Team → Application → ApiExposure dependency chain.
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
			SetName("consumer-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appID = app.ID

		// Seed a target ApiExposure for a different application (provider).
		providerApp, err := client.Application.Create().
			SetName("provider-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetBasePath("/api/v1/users").
			SetNamespace("platform--narvi").
			SetVisibility(entapiexposure.VisibilityWorld).
			SetActive(true).
			SetFeatures([]string{}).
			SetOwnerID(providerApp.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		exposureID = exposure.ID

		deps = &mockSubscriptionDeps{
			appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
			exposureIDs: map[string]int{"/api/v1/users": exposureID},
		}

		repo = apisubscription.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	baseData := func() *apisubscription.APISubscriptionData {
		return &apisubscription.APISubscriptionData{
			Meta: shared.Metadata{
				Namespace:   "prod--platform--narvi",
				Name:        "my-subscription",
				Environment: "prod",
			},
			StatusPhase:    "READY",
			StatusMessage:  "subscription active",
			BasePath:       "/api/v1/users",
			M2MAuthMethod:  "OAUTH2_CLIENT",
			ApprovedScopes: []string{"read", "write"},
			OwnerAppName:   "consumer-app",
			OwnerTeamName:  "platform--narvi",
			TargetBasePath: "/api/v1/users",
			TargetAppName:  "",
			TargetTeamName: "",
		}
	}

	Describe("Upsert", func() {
		It("should create a new subscription with valid target exposure FK", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify the subscription was created.
			sub, err := client.ApiSubscription.Query().
				Where(
					entapisub.BasePathEQ("/api/v1/users"),
					entapisub.HasOwnerWith(),
				).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.BasePath).To(Equal("/api/v1/users"))
			Expect(sub.M2mAuthMethod.String()).To(Equal("OAUTH2_CLIENT"))
			Expect(sub.ApprovedScopes).To(Equal([]string{"read", "write"}))
			Expect(sub.StatusPhase.String()).To(Equal("READY"))
			Expect(*sub.StatusMessage).To(Equal("subscription active"))

			// Verify target FK is set.
			targetFK := sub.Edges.Target
			// Query edges to check target.
			sub2, err := client.ApiSubscription.Query().
				Where(entapisub.IDEQ(sub.ID)).
				WithTarget().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			_ = targetFK // edges not loaded in first query
			Expect(sub2.Edges.Target).NotTo(BeNil())
			Expect(sub2.Edges.Target.ID).To(Equal(exposureID))
		})

		It("should create a subscription with nil target FK when exposure is missing", func() {
			// Override deps so exposure lookup fails.
			missingDeps := &mockSubscriptionDeps{
				appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
				exposureIDs: map[string]int{}, // empty — no exposure found
			}
			repo = apisubscription.NewRepository(client, cache, missingDeps)

			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify target FK is nil.
			sub, err := client.ApiSubscription.Query().
				Where(entapisub.BasePathEQ("/api/v1/users")).
				WithTarget().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.Edges.Target).To(BeNil())
		})

		It("should return ErrDependencyMissing when owner application is missing", func() {
			missingDeps := &mockSubscriptionDeps{
				appIDs:      map[string]int{}, // empty — no app found
				exposureIDs: map[string]int{},
			}
			repo = apisubscription.NewRepository(client, cache, missingDeps)

			data := baseData()
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("should propagate non-ErrEntityNotFound errors from FindApplicationID", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockSubscriptionDeps{
				appIDs:      map[string]int{},
				exposureIDs: map[string]int{},
				appErr:      dbErr,
			}
			failRepo := apisubscription.NewRepository(client, cache, failDeps)

			data := baseData()
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should clear target FK when target exposure is removed", func() {
			// First upsert with target.
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify first subscription exists with target.
			sub1, err := client.ApiSubscription.Query().
				Where(entapisub.BasePathEQ("/api/v1/users")).
				WithTarget().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub1.Edges.Target).NotTo(BeNil())
			originalID := sub1.ID

			// Second upsert with no target (exposure gone).
			missingDeps := &mockSubscriptionDeps{
				appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
				exposureIDs: map[string]int{}, // empty — target removed
			}
			repo = apisubscription.NewRepository(client, cache, missingDeps)

			data2 := baseData()
			data2.StatusMessage = "waiting for target"
			Expect(repo.Upsert(ctx, data2)).To(Succeed())

			// Verify only one subscription exists.
			subs, err := client.ApiSubscription.Query().
				Where(entapisub.BasePathEQ("/api/v1/users")).
				All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(subs).To(HaveLen(1))
			Expect(*subs[0].StatusMessage).To(Equal("waiting for target"))

			// Verify the row was updated in-place (same ID, no delete+recreate).
			Expect(subs[0].ID).To(Equal(originalID))

			// Verify target FK is now nil.
			sub2, err := client.ApiSubscription.Query().
				Where(entapisub.BasePathEQ("/api/v1/users")).
				WithTarget().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub2.Edges.Target).To(BeNil())
		})

		It("should update an existing subscription on conflict", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update status.
			data.StatusPhase = "ERROR"
			data.StatusMessage = "failed to connect"
			data.M2MAuthMethod = "BASIC_AUTH"
			data.ApprovedScopes = []string{}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err := client.ApiSubscription.Query().
				Where(entapisub.BasePathEQ("/api/v1/users")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.StatusPhase.String()).To(Equal("ERROR"))
			Expect(*sub.StatusMessage).To(Equal("failed to connect"))
			Expect(sub.M2mAuthMethod.String()).To(Equal("BASIC_AUTH"))
			Expect(sub.ApprovedScopes).To(Equal([]string{}))
		})

		It("should maintain meta cache entry", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			// Verify meta cache key.
			metaKey := "meta:prod--platform--narvi:my-subscription"
			metaID, metaOK := cache.Get("apisubscription", metaKey)
			Expect(metaOK).To(BeTrue())
			Expect(metaID).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing subscription and clean meta cache entry", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := apisubscription.APISubscriptionKey{
				BasePath:      "/api/v1/users",
				OwnerAppName:  "consumer-app",
				OwnerTeamName: "platform--narvi",
				Namespace:     "prod--platform--narvi",
				Name:          "my-subscription",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Verify deleted from DB.
			count, err := client.ApiSubscription.Query().
				Where(entapisub.BasePathEQ("/api/v1/users")).
				Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			// Verify meta cache cleaned.
			_, ok := cache.Get("apisubscription", "meta:prod--platform--narvi:my-subscription")
			Expect(ok).To(BeFalse())
		})

		It("should be idempotent — deleting a non-existent subscription succeeds", func() {
			key := apisubscription.APISubscriptionKey{
				BasePath:      "/api/v1/nonexistent",
				OwnerAppName:  "consumer-app",
				OwnerTeamName: "platform--narvi",
				Namespace:     "ns",
				Name:          "n",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should not clean meta cache when namespace/name are empty", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			// Delete without namespace/name — simulates best-effort fallback.
			key := apisubscription.APISubscriptionKey{
				BasePath:      "/api/v1/users",
				OwnerAppName:  "consumer-app",
				OwnerTeamName: "platform--narvi",
				Namespace:     "",
				Name:          "",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Meta cache is NOT cleaned (namespace/name empty).
			metaID, metaOK := cache.Get("apisubscription", "meta:prod--platform--narvi:my-subscription")
			Expect(metaOK).To(BeTrue())
			Expect(metaID).To(BeNumerically(">", 0))
		})
	})
})
