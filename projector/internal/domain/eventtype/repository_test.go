// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype_test

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	enteventtype "github.com/telekom/controlplane/controlplane-api/ent/eventtype"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/domain/eventtype"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockEventTypeDeps implements eventtype.EventTypeDeps for testing.
type mockEventTypeDeps struct {
	teamIDs map[string]int // key: team name
	teamErr error          // if non-nil, FindTeamID always returns this error
}

func (m *mockEventTypeDeps) FindTeamID(_ context.Context, name string) (int, error) {
	if m.teamErr != nil {
		return 0, m.teamErr
	}
	if id, ok := m.teamIDs[name]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("team %q: %w", name, infrastructure.ErrEntityNotFound)
}

var _ = Describe("EventType Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockEventTypeDeps
		repo   *eventtype.Repository
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

		deps = &mockEventTypeDeps{
			teamIDs: map[string]int{"platform--narvi": teamID},
		}
		repo = eventtype.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an event_type with valid deps", func() {
			data := &eventtype.EventTypeData{
				Meta:          shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase:   "READY",
				StatusMessage: "ok",
				EventType:     "weather.forecast.updated",
				Version:       "1.0.0",
				Description:   "Weather forecast updated",
				Active:        true,
				TeamName:      "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			et, err := client.EventType.Query().
				Where(enteventtype.EventTypeEQ("weather.forecast.updated")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(et.EventType).To(Equal("weather.forecast.updated"))
			Expect(et.Version).To(Equal("1.0.0"))
			Expect(et.Description).To(Equal("Weather forecast updated"))
			Expect(et.Active).To(BeTrue())

			owner, err := et.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(teamID))
		})

		It("should return ErrDependencyMissing when the owner Team is missing", func() {
			deps.teamIDs = map[string]int{}
			data := &eventtype.EventTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase: "READY",
				EventType:   "weather.forecast.updated",
				Version:     "1.0.0",
				TeamName:    "platform--narvi",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should update an existing event_type on conflict", func() {
			data := &eventtype.EventTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase: "READY",
				EventType:   "weather.forecast.updated",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Version = "2.0.0"
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			count, err := client.EventType.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			et, err := client.EventType.Query().
				Where(enteventtype.EventTypeEQ("weather.forecast.updated")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(et.Version).To(Equal("2.0.0"))
		})

		It("should set the active-eventtype cache entry when active", func() {
			data := &eventtype.EventTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase: "READY",
				EventType:   "weather.forecast.updated",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			resolver := infrastructure.NewIDResolver(client, cache)
			id, err := resolver.FindActiveEventTypeID(ctx, "weather.forecast.updated")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))
		})

		It("should clear the active-eventtype cache entry when inactive", func() {
			data := &eventtype.EventTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase: "READY",
				EventType:   "weather.forecast.updated",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			data.Active = false
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err := resolver.FindActiveEventTypeID(ctx, "weather.forecast.updated")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})

		It("should back-link orphaned EventExposures projected before the event_type", func() {
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

			// EventExposure created first, before its EventType exists → stored
			// with a NULL event_type_def FK (the create-order race).
			exp, err := client.EventExposure.Create().
				SetEventType("weather.forecast.updated").
				SetNamespace("prod--platform--narvi").
				SetActive(true).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = exp.QueryEventTypeDef().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())

			// EventType appears later → should adopt the orphaned exposure.
			data := &eventtype.EventTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase: "READY",
				EventType:   "weather.forecast.updated",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			linked, err := exp.QueryEventTypeDef().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(linked.EventType).To(Equal("weather.forecast.updated"))
		})

		It("should not back-link EventExposures when the EventType is inactive", func() {
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

			exp, err := client.EventExposure.Create().
				SetEventType("weather.forecast.updated").
				SetNamespace("prod--platform--narvi").
				SetActive(true).
				SetOwnerID(app.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			data := &eventtype.EventTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase: "READY",
				EventType:   "weather.forecast.updated",
				Version:     "1.0.0",
				Active:      false,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			_, err = exp.QueryEventTypeDef().Only(ctx)
			Expect(ent.IsNotFound(err)).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should be idempotent when the entity does not exist", func() {
			key := eventtype.EventTypeKey{EventType: "missing.event", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})

		It("should delete an existing event_type by event type and team name", func() {
			data := &eventtype.EventTypeData{
				Meta:        shared.NewMetadata("prod--platform--narvi", "evt-weather-v1", nil),
				StatusPhase: "READY",
				EventType:   "weather.forecast.updated",
				Version:     "1.0.0",
				Active:      true,
				TeamName:    "platform--narvi",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := eventtype.EventTypeKey{EventType: "weather.forecast.updated", TeamName: "platform--narvi"}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.EventType.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			resolver := infrastructure.NewIDResolver(client, cache)
			_, err = resolver.FindActiveEventTypeID(ctx, "weather.forecast.updated")
			Expect(errors.Is(err, infrastructure.ErrEntityNotFound)).To(BeTrue())
		})
	})
})
