// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription_test

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
	entagenticsub "github.com/telekom/controlplane/controlplane-api/ent/agenticsubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	"github.com/telekom/controlplane/projector/internal/domain/agenticsubscription"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockSubscriptionDeps implements agenticsubscription.AgenticSubscriptionDeps for testing.
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

func (m *mockSubscriptionDeps) FindAgenticExposureByBasePath(_ context.Context, basePath string) (int, error) {
	if id, ok := m.exposureIDs[basePath]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("agentic_exposure basePath %q: %w", basePath, infrastructure.ErrEntityNotFound)
}

func (m *mockSubscriptionDeps) EvictAgenticExposureByBasePath(basePath string) {
	delete(m.exposureIDs, basePath)
}

var _ = Describe("AgenticSubscription Repository", func() {
	var (
		client     *ent.Client
		cache      *infrastructure.EdgeCache
		deps       *mockSubscriptionDeps
		repo       *agenticsubscription.Repository
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

		// Seed Zone → Team → Application → AgenticExposure dependency chain.
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

		// Seed a target AgenticExposure for a different application (provider).
		providerApp, err := client.Application.Create().
			SetName("provider-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.AgenticExposure.Create().
			SetBasePath("/mcp/v1/tools").
			SetNamespace("platform--narvi").
			SetVisibility(entagenticexposure.VisibilityWorld).
			SetVariant(entagenticexposure.VariantMcp).
			SetActive(true).
			SetUpstreams([]model.Upstream{}).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			SetOwnerID(providerApp.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		exposureID = exposure.ID

		deps = &mockSubscriptionDeps{
			appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
			exposureIDs: map[string]int{"/mcp/v1/tools": exposureID},
		}

		repo = agenticsubscription.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	baseData := func() *agenticsubscription.AgenticSubscriptionData {
		return &agenticsubscription.AgenticSubscriptionData{
			Meta: shared.Metadata{
				Namespace:   "prod--platform--narvi",
				Name:        "my-subscription",
				Environment: "prod",
			},
			StatusPhase:   "READY",
			StatusMessage: "subscription active",
			BasePath:      "/mcp/v1/tools",
			Security: &model.AgenticSubscriptionSecurity{
				M2M: &model.SubscriberMachine2MachineAuthentication{
					Client: &model.OAuth2ClientCredentials{
						ClientId: "my-client-id",
					},
					Scopes: []string{"read", "write"},
				},
			},
			Traffic: &model.AgenticSubscriberTraffic{
				Failover: &model.AgenticSubscriberFailover{
					Enabled: true,
				},
			},
			OwnerAppName:   "consumer-app",
			OwnerTeamName:  "platform--narvi",
			TargetBasePath: "/mcp/v1/tools",
		}
	}

	Describe("Upsert", func() {
		It("should create a new subscription with valid target exposure FK", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify the subscription was created.
			sub, err := client.AgenticSubscription.Query().
				Where(
					entagenticsub.BasePathEQ("/mcp/v1/tools"),
					entagenticsub.HasOwnerWith(),
				).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.BasePath).To(Equal("/mcp/v1/tools"))
			Expect(sub.StatusPhase.String()).To(Equal("READY"))
			Expect(*sub.StatusMessage).To(Equal("subscription active"))
			Expect(sub.Security.M2M).NotTo(BeNil())
			Expect(sub.Security.M2M.Client).NotTo(BeNil())
			Expect(sub.Security.M2M.Client.ClientId).To(Equal("my-client-id"))
			Expect(sub.Traffic.Failover).NotTo(BeNil())
			Expect(sub.Traffic.Failover.Enabled).To(BeTrue())

			// Verify target FK is set.
			sub2, err := client.AgenticSubscription.Query().
				Where(entagenticsub.IDEQ(sub.ID)).
				WithTarget().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub2.Edges.Target).NotTo(BeNil())
			Expect(sub2.Edges.Target.ID).To(Equal(exposureID))
		})

		It("should create a subscription with nil target FK when exposure is missing", func() {
			// Override deps so exposure lookup fails.
			missingDeps := &mockSubscriptionDeps{
				appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
				exposureIDs: map[string]int{}, // empty — no exposure found
			}
			repo = agenticsubscription.NewRepository(client, cache, missingDeps)

			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify target FK is nil.
			sub, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
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
			repo = agenticsubscription.NewRepository(client, cache, missingDeps)

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
			failRepo := agenticsubscription.NewRepository(client, cache, failDeps)

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
			sub1, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
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
			repo = agenticsubscription.NewRepository(client, cache, missingDeps)

			data2 := baseData()
			data2.StatusMessage = "waiting for target"
			Expect(repo.Upsert(ctx, data2)).To(Succeed())

			// Verify only one subscription exists.
			subs, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
				All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(subs).To(HaveLen(1))
			Expect(*subs[0].StatusMessage).To(Equal("waiting for target"))

			// Verify the row was updated in-place (same ID, no delete+recreate).
			Expect(subs[0].ID).To(Equal(originalID))

			// Verify target FK is now nil.
			sub2, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
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
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.StatusPhase.String()).To(Equal("ERROR"))
			Expect(*sub.StatusMessage).To(Equal("failed to connect"))
		})

		It("should update security- and traffic-derived fields on upsert conflict", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.Security.M2M.Client).NotTo(BeNil())

			// Change to basic auth security and disable failover.
			data.Security = &model.AgenticSubscriptionSecurity{
				M2M: &model.SubscriberMachine2MachineAuthentication{
					Basic: &model.BasicAuthCredentials{
						Username: "test-user",
						Password: "test-dummy-pass",
					},
					Scopes: []string{"admin"},
				},
			}
			data.Traffic = &model.AgenticSubscriberTraffic{
				Failover: &model.AgenticSubscriberFailover{Enabled: false},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err = client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.Security.M2M.Client).To(BeNil())
			Expect(sub.Security.M2M.Basic).NotTo(BeNil())
			Expect(sub.Security.M2M.Basic.Username).To(Equal("test-user"))
			Expect(sub.Traffic.Failover.Enabled).To(BeFalse())

			// Clear security and traffic — set to nil.
			data.Security = nil
			data.Traffic = nil
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err = client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.Security.M2M).To(BeNil())
			Expect(sub.Traffic.Failover).To(BeNil())
		})

		It("should return an error when the cached target exposure ID is stale", func() {
			// Cached exposure ID points to a row that does not exist. On a
			// Postgres backend this surfaces as an FK violation (23503) which
			// the repository maps to ErrDependencyMissing after evicting the
			// stale cache entry (see IsFKViolation + fkTargetExposure).
			// ponytail: the repo test harness is SQLite, whose FK error is not
			// a *pgconn.PgError, so the pg-specific remap branch cannot fire
			// here; the constraint-name matching itself is unit-tested in
			// infrastructure/errors_test.go. We assert the stale ID is not
			// silently persisted.
			staleDeps := &mockSubscriptionDeps{
				appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
				exposureIDs: map[string]int{"/mcp/v1/tools": exposureID + 9999}, // nonexistent
			}
			repo = agenticsubscription.NewRepository(client, cache, staleDeps)

			err := repo.Upsert(ctx, baseData())
			Expect(err).To(HaveOccurred())

			// The bad target FK must not have been persisted.
			count, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
				Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should maintain meta cache entry", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			// Verify meta cache key.
			metaKey := "meta:prod--platform--narvi:my-subscription"
			metaID, metaOK := cache.Get("agenticsubscription", metaKey)
			Expect(metaOK).To(BeTrue())
			Expect(metaID).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing subscription and clean meta cache entry", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := agenticsubscription.AgenticSubscriptionKey{
				BasePath:      "/mcp/v1/tools",
				OwnerAppName:  "consumer-app",
				OwnerTeamName: "platform--narvi",
				Namespace:     "prod--platform--narvi",
				Name:          "my-subscription",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Verify deleted from DB.
			count, err := client.AgenticSubscription.Query().
				Where(entagenticsub.BasePathEQ("/mcp/v1/tools")).
				Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			// Verify meta cache cleaned.
			_, ok := cache.Get("agenticsubscription", "meta:prod--platform--narvi:my-subscription")
			Expect(ok).To(BeFalse())
		})

		It("should be idempotent — deleting a non-existent subscription succeeds", func() {
			key := agenticsubscription.AgenticSubscriptionKey{
				BasePath:      "/mcp/v1/nonexistent",
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
			key := agenticsubscription.AgenticSubscriptionKey{
				BasePath:      "/mcp/v1/tools",
				OwnerAppName:  "consumer-app",
				OwnerTeamName: "platform--narvi",
				Namespace:     "",
				Name:          "",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Meta cache is NOT cleaned (namespace/name empty).
			metaID, metaOK := cache.Get("agenticsubscription", "meta:prod--platform--narvi:my-subscription")
			Expect(metaOK).To(BeTrue())
			Expect(metaID).To(BeNumerically(">", 0))
		})
	})
})
