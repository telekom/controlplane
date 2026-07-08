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
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	enteventexposure "github.com/telekom/controlplane/controlplane-api/ent/eventexposure"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	"github.com/telekom/controlplane/projector/internal/domain/eventexposure"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
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
				AppName:  "my-app",
				TeamName: "platform--narvi",
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
