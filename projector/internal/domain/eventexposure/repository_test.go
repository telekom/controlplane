// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	enteventexposure "github.com/telekom/controlplane/controlplane-api/ent/eventexposure"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	"github.com/telekom/controlplane/projector/internal/domain/eventexposure"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockExposureDeps implements eventexposure.EventExposureDeps for testing.
type mockExposureDeps struct {
	appIDs          map[string]int
	appErr          error
	activeEvtTypeID map[string]int // key: eventType
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

func (m *mockExposureDeps) FindActiveEventTypeID(_ context.Context, eventType string) (int, error) {
	if m.activeEvtTypeID != nil {
		if id, ok := m.activeEvtTypeID[eventType]; ok {
			return id, nil
		}
	}
	return 0, fmt.Errorf("active event_type %q: %w", eventType, infrastructure.ErrEntityNotFound)
}

var _ = Describe("EventExposure Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockExposureDeps
		repo   *eventexposure.Repository
		ctx    context.Context
		appID  int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

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
		repo = eventexposure.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an event exposure with valid deps", func() {
			data := &eventexposure.EventExposureData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "exp-1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				EventType:     "de.telekom.eni.quickstart.v1",
				Visibility:    "WORLD",
				Active:        true,
				ApprovalConfig: model.ApprovalConfig{
					Strategy:     "AUTO",
					TrustedTeams: []string{"team-a"},
				},
				Scopes: []model.EventScope{
					{
						Name: "my-scope",
						Trigger: model.EventTrigger{
							ResponseFilter: &model.ResponseFilter{
								Paths: []string{"$.data.id", "$.data.name"},
								Mode:  "Include",
							},
							SelectionFilter: &model.SelectionFilter{
								Attributes: map[string]string{"type": "de.telekom.eni.quickstart.v1"},
								Expression: `{"op":"eq","path":"$.source","value":"my-app"}`,
							},
						},
					},
				},
				GatewayProviderUrl: "https://publish.gateway.example.com/de.telekom.eni.quickstart.v1",
				AppName:            "my-app",
				TeamName:           "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.EventExposure.Query().
				Where(enteventexposure.EventTypeEQ("de.telekom.eni.quickstart.v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(exp.Visibility.String()).To(Equal("WORLD"))
			Expect(exp.Active).ToNot(BeNil())
			Expect(*exp.Active).To(BeTrue())
			Expect(exp.ApprovalConfig.Strategy).To(Equal("AUTO"))

			owner, err := exp.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(appID))

			Expect(exp.EventScopes).To(HaveLen(1))
			Expect(exp.EventScopes[0].Name).To(Equal("my-scope"))
			Expect(exp.EventScopes[0].Trigger.ResponseFilter).NotTo(BeNil())
			Expect(exp.EventScopes[0].Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.id", "$.data.name"}))
			Expect(exp.EventScopes[0].Trigger.ResponseFilter.Mode).To(Equal("Include"))
			Expect(exp.EventScopes[0].Trigger.SelectionFilter).NotTo(BeNil())
			Expect(exp.EventScopes[0].Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"type": "de.telekom.eni.quickstart.v1"}))
			Expect(exp.EventScopes[0].Trigger.SelectionFilter.Expression).To(Equal(`{"op":"eq","path":"$.source","value":"my-app"}`))

			Expect(exp.GatewayPublishingURL).NotTo(BeNil())
			Expect(*exp.GatewayPublishingURL).To(Equal("https://publish.gateway.example.com/de.telekom.eni.quickstart.v1"))
		})

		It("should back-link orphaned subscriptions projected before the exposure", func() {
			// Subscription created first, before its target exposure exists →
			// stored with a NULL target FK (the create-order race).
			sub, err := client.EventSubscription.Create().
				SetEventType("de.telekom.orphan.v1").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("orphan-sub").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			// Exposure appears later → should adopt the orphaned subscription.
			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "orphan-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				EventType:      "de.telekom.orphan.v1",
				Visibility:     "WORLD",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				Scopes:         []model.EventScope{},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			target, err := sub.QueryTarget().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(target.EventType).To(Equal("de.telekom.orphan.v1"))
		})

		It("should not back-link subscriptions when the exposure is inactive", func() {
			sub, err := client.EventSubscription.Create().
				SetEventType("de.telekom.inactive.v1").
				SetEnvironment("prod").
				SetNamespace("prod--platform--narvi").
				SetName("inactive-sub").
				SetOwnerID(appID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "inactive-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				EventType:      "de.telekom.inactive.v1",
				Visibility:     "WORLD",
				Active:         false,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				Scopes:         []model.EventScope{},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			_, err = sub.QueryTarget().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should clear a stale event_type_def FK once the EventType is no longer resolvable", func() {
			catalogueTeam, err := client.Team.Create().
				SetName("platform--catalogue").
				SetEmail("catalogue@example.com").
				SetNamespace("platform--catalogue").
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			catalogueEvtType, err := client.EventType.Create().
				SetEventType("de.telekom.stale.v1").
				SetNamespace("platform--catalogue").
				SetVersion("1.0.0").
				SetActive(true).
				SetOwnerID(catalogueTeam.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "stale-exp", nil),
				StatusPhase:    "READY",
				StatusMessage:  "ok",
				EventType:      "de.telekom.stale.v1",
				Visibility:     "WORLD",
				Active:         true,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				Scopes:         []model.EventScope{},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			deps.activeEvtTypeID = map[string]int{"de.telekom.stale.v1": catalogueEvtType.ID}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.EventExposure.Query().
				Where(enteventexposure.EventTypeEQ("de.telekom.stale.v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			linked, err := exp.QueryEventTypeDef().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linked.ID).To(Equal(catalogueEvtType.ID))

			// EventType becomes unresolvable (e.g. it went inactive) — re-reconcile.
			deps.activeEvtTypeID = map[string]int{}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err = client.EventExposure.Query().
				Where(enteventexposure.EventTypeEQ("de.telekom.stale.v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = exp.QueryEventTypeDef().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})

		It("should return ErrDependencyMissing when application is missing", func() {
			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "fail-exp", nil),
				StatusPhase:    "UNKNOWN",
				EventType:      "de.telekom.fail.v1",
				Visibility:     "ENTERPRISE",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "missing-app",
				TeamName:       "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should propagate non-ErrEntityNotFound errors from FindApplicationID", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockExposureDeps{appErr: dbErr}
			failRepo := eventexposure.NewRepository(client, cache, failDeps)

			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "fail-exp", nil),
				StatusPhase:    "UNKNOWN",
				EventType:      "de.telekom.fail.v1",
				Visibility:     "ENTERPRISE",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should update existing exposure on conflict", func() {
			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "upd-exp", nil),
				StatusPhase:    "PENDING",
				StatusMessage:  "v1",
				EventType:      "de.telekom.update.v1",
				Visibility:     "ENTERPRISE",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.StatusPhase = "READY"
			data.Visibility = "WORLD"
			data.Active = true
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err := client.EventExposure.Query().
				Where(enteventexposure.EventTypeEQ("de.telekom.update.v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.Visibility.String()).To(Equal("WORLD"))
			Expect(*exp.Active).To(BeTrue())
		})

		It("should populate the edge cache after upsert", func() {
			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "cached-exp", nil),
				StatusPhase:    "READY",
				EventType:      "de.telekom.cached.v1",
				Visibility:     "ENTERPRISE",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("eventexposure", "de.telekom.cached.v1:my-app:platform--narvi")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})

		It("should replace response filter with selection filter on update", func() {
			data := &eventexposure.EventExposureData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "replace-exp", nil),
				StatusPhase: "READY",
				EventType:   "de.telekom.replace.v1",
				Visibility:  "WORLD",
				Active:      true,
				ApprovalConfig: model.ApprovalConfig{
					Strategy: "AUTO",
				},
				Scopes: []model.EventScope{
					{
						Name: "filter-scope",
						Trigger: model.EventTrigger{
							ResponseFilter: &model.ResponseFilter{
								Paths: []string{"$.data.secret"},
								Mode:  "Exclude",
							},
						},
					},
				},
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify initial state has response filter
			exp, err := client.EventExposure.Query().
				Where(enteventexposure.EventTypeEQ("de.telekom.replace.v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.EventScopes).To(HaveLen(1))
			Expect(exp.EventScopes[0].Name).To(Equal("filter-scope"))
			Expect(exp.EventScopes[0].Trigger.ResponseFilter).NotTo(BeNil())
			Expect(exp.EventScopes[0].Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.secret"}))
			Expect(exp.EventScopes[0].Trigger.ResponseFilter.Mode).To(Equal("Exclude"))
			Expect(exp.EventScopes[0].Trigger.SelectionFilter).To(BeNil())

			// Update: replace response filter with selection filter
			data.Scopes = []model.EventScope{
				{
					Name: "filter-scope",
					Trigger: model.EventTrigger{
						SelectionFilter: &model.SelectionFilter{
							Attributes: map[string]string{"source": "my-service"},
							Expression: `{"op":"eq","path":"$.type","value":"order.created"}`,
						},
					},
				},
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			exp, err = client.EventExposure.Query().
				Where(enteventexposure.EventTypeEQ("de.telekom.replace.v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(exp.EventScopes).To(HaveLen(1))
			Expect(exp.EventScopes[0].Name).To(Equal("filter-scope"))
			Expect(exp.EventScopes[0].Trigger.ResponseFilter).To(BeNil())
			Expect(exp.EventScopes[0].Trigger.SelectionFilter).NotTo(BeNil())
			Expect(exp.EventScopes[0].Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"source": "my-service"}))
			Expect(exp.EventScopes[0].Trigger.SelectionFilter.Expression).To(Equal(`{"op":"eq","path":"$.type","value":"order.created"}`))
		})

	})

	Describe("active event-type lookup cache invalidation", func() {
		const ident = "de.telekom.active.lookup.v1"
		var resolver *infrastructure.IDResolver

		exposure := func(appName string, active bool) *eventexposure.EventExposureData {
			return &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "exp-"+appName, nil),
				StatusPhase:    "READY",
				EventType:      ident,
				Visibility:     "ENTERPRISE",
				Active:         active,
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        appName,
				TeamName:       "platform--narvi",
			}
		}
		key := eventexposure.EventExposureKey{EventType: ident, AppName: "my-app", TeamName: "platform--narvi"}

		// primeActive resolves the active exposure through the real resolver and
		// proves the resolved ID is now served from the edge cache.
		primeActive := func() int {
			id, err := resolver.FindEventExposureByEventType(ctx, ident)
			Expect(err).NotTo(HaveOccurred())
			cache.Wait()
			cached, found := cache.Get(cachekeys.EventExposureByEventType(ident))
			Expect(found).To(BeTrue())
			Expect(cached).To(Equal(id))
			return id
		}

		BeforeEach(func() {
			resolver = infrastructure.NewIDResolver(client, cache)
			Expect(repo.Upsert(ctx, exposure("my-app", true))).To(Succeed())
		})

		It("stops resolving an exposure once it is deactivated", func() {
			primeActive()
			Expect(repo.Upsert(ctx, exposure("my-app", false))).To(Succeed())
			cache.Wait()

			_, err := resolver.FindEventExposureByEventType(ctx, ident)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})

		It("stops resolving an exposure once it is deleted, also on idempotent re-delete", func() {
			primeActive()
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()
			_, err := resolver.FindEventExposureByEventType(ctx, ident)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			// A stale active entry left behind must be evicted by a count=0 delete.
			et, lk := cachekeys.EventExposureByEventType(ident)
			cache.Set(et, lk, 4242)
			cache.Wait()
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()
			_, found := cache.Get(et, lk)
			Expect(found).To(BeFalse())
		})

		It("resolves the newly active owner instead of the stale inactive one", func() {
			other, err := client.Application.Create().
				SetName("other-app").
				SetNamespace("platform--narvi").
				SetOwnerTeamID(client.Team.Query().OnlyIDX(ctx)).
				SetZoneID(client.Zone.Query().OnlyIDX(ctx)).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			deps.appIDs["other-app:platform--narvi"] = other.ID

			oldID := primeActive()
			Expect(repo.Upsert(ctx, exposure("my-app", false))).To(Succeed())
			Expect(repo.Upsert(ctx, exposure("other-app", true))).To(Succeed())
			cache.Wait()

			id, err := resolver.FindEventExposureByEventType(ctx, ident)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(Equal(oldID))
			Expect(id).To(Equal(client.EventExposure.Query().
				Where(enteventexposure.HasOwnerWith(application.IDEQ(other.ID))).OnlyIDX(ctx)))
		})

		// failOn makes every ENT mutation of the given operation fail
		// deterministically, simulating a DB error at that write.
		failOn := func(op ent.Op) error {
			injected := errors.New("injected db failure")
			client.EventExposure.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
					if m.Op().Is(op) {
						return nil, injected
					}
					return next.Mutate(ctx, m)
				})
			})
			return injected
		}

		It("keeps the cached active lookup when the primary upsert fails", func() {
			id := primeActive()
			injected := failOn(ent.OpCreate)

			Expect(errors.Is(repo.Upsert(ctx, exposure("my-app", false)), injected)).To(BeTrue())
			cache.Wait()
			cached, found := cache.Get(cachekeys.EventExposureByEventType(ident))
			Expect(found).To(BeTrue())
			Expect(cached).To(Equal(id))
		})

		It("evicts the active lookup when a write after the primary upsert fails", func() {
			primeActive()
			// The catalogue FK update (UpdateOne) runs after the primary upsert.
			injected := failOn(ent.OpUpdateOne)

			Expect(errors.Is(repo.Upsert(ctx, exposure("my-app", false)), injected)).To(BeTrue())
			cache.Wait()
			Expect(client.EventExposure.Query().Where(enteventexposure.ActiveEQ(false)).CountX(ctx)).To(Equal(1))
			_, err := resolver.FindEventExposureByEventType(ctx, ident)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})

		It("keeps the cached active lookup when the delete fails", func() {
			id := primeActive()
			injected := failOn(ent.OpDelete)

			Expect(errors.Is(repo.Delete(ctx, key), injected)).To(BeTrue())
			cache.Wait()
			cached, found := cache.Get(cachekeys.EventExposureByEventType(ident))
			Expect(found).To(BeTrue())
			Expect(cached).To(Equal(id))
		})

		It("resolves a reactivated exposure once no negative entry applies", func() {
			id := primeActive()
			Expect(repo.Upsert(ctx, exposure("my-app", false))).To(Succeed())
			cache.Wait()
			_, err := resolver.FindEventExposureByEventType(ctx, ident)
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())

			Expect(repo.Upsert(ctx, exposure("my-app", true))).To(Succeed())
			cache.Wait()
			// The original resolver keeps its bounded negative entry; a fresh
			// resolver sharing the edge cache reads the reactivated row.
			fresh := infrastructure.NewIDResolver(client, cache)
			Expect(fresh.FindEventExposureByEventType(ctx, ident)).To(Equal(id))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing event exposure", func() {
			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "del-exp", nil),
				StatusPhase:    "READY",
				EventType:      "de.telekom.delete.v1",
				Visibility:     "ENTERPRISE",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := eventexposure.EventExposureKey{
				EventType: "de.telekom.delete.v1",
				AppName:   "my-app",
				TeamName:  "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.EventExposure.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should be idempotent for non-existent exposure", func() {
			key := eventexposure.EventExposureKey{
				EventType: "de.telekom.nonexistent.v1",
				AppName:   "my-app",
				TeamName:  "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should evict from edge cache after delete", func() {
			data := &eventexposure.EventExposureData{
				Meta:           shared.NewMetadata("prod--platform--narvi", "evict-exp", nil),
				StatusPhase:    "READY",
				EventType:      "de.telekom.evict.v1",
				Visibility:     "ENTERPRISE",
				ApprovalConfig: model.ApprovalConfig{Strategy: "AUTO"},
				AppName:        "my-app",
				TeamName:       "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			_, found := cache.Get("eventexposure", "de.telekom.evict.v1:my-app:platform--narvi")
			Expect(found).To(BeTrue())

			key := eventexposure.EventExposureKey{
				EventType: "de.telekom.evict.v1",
				AppName:   "my-app",
				TeamName:  "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
			cache.Wait()

			_, found = cache.Get("eventexposure", "de.telekom.evict.v1:my-app:platform--narvi")
			Expect(found).To(BeFalse())
		})
	})
})
