// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription_test

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
	enteventsubscription "github.com/telekom/controlplane/controlplane-api/ent/eventsubscription"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"

	"github.com/telekom/controlplane/projector/internal/domain/eventsubscription"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockSubscriptionDeps implements eventsubscription.EventSubscriptionDeps for testing.
type mockSubscriptionDeps struct {
	appIDs      map[string]int
	appErr      error
	exposureIDs map[string]int
	exposureErr error
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

func (m *mockSubscriptionDeps) FindEventExposureByEventType(_ context.Context, eventType string) (int, error) {
	if m.exposureErr != nil {
		return 0, m.exposureErr
	}
	if id, ok := m.exposureIDs[eventType]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("event_exposure %q: %w", eventType, infrastructure.ErrEntityNotFound)
}

var _ = Describe("EventSubscription Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockSubscriptionDeps
		repo   *eventsubscription.Repository
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
			SetName("consumer-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appID = app.ID

		deps = &mockSubscriptionDeps{
			appIDs:      map[string]int{"consumer-app:platform--narvi": appID},
			exposureIDs: map[string]int{},
		}
		repo = eventsubscription.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	Describe("Upsert", func() {
		It("should create an event subscription with valid deps (no target)", func() {
			callbackURL := "https://consumer.example.com/events"
			data := &eventsubscription.EventSubscriptionData{
				Meta:            shared.NewMetadata("prod--platform--narvi", "sub-1", nil),
				StatusPhase:     "READY",
				StatusMessage:   "ok",
				EventType:       "de.telekom.eni.quickstart.v1",
				DeliveryType:    "CALLBACK",
				CallbackURL:     &callbackURL,
				OwnerAppName:    "consumer-app",
				OwnerTeamName:   "platform--narvi",
				TargetEventType: "de.telekom.eni.quickstart.v1",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			sub, err := client.EventSubscription.Query().
				Where(enteventsubscription.EventTypeEQ("de.telekom.eni.quickstart.v1")).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(sub.DeliveryType.String()).To(Equal("CALLBACK"))
			Expect(sub.CallbackURL).ToNot(BeNil())
			Expect(*sub.CallbackURL).To(Equal("https://consumer.example.com/events"))

			owner, err := sub.QueryOwner().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.ID).To(Equal(appID))

			// Target should be nil (no exposure found).
			hasTarget, err := sub.QueryTarget().Exist(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasTarget).To(BeFalse())
		})

		It("should return ErrDependencyMissing when application is missing", func() {
			data := &eventsubscription.EventSubscriptionData{
				Meta:            shared.NewMetadata("prod--platform--narvi", "fail-sub", nil),
				StatusPhase:     "UNKNOWN",
				EventType:       "de.telekom.fail.v1",
				DeliveryType:    "CALLBACK",
				OwnerAppName:    "missing-app",
				OwnerTeamName:   "platform--narvi",
				TargetEventType: "de.telekom.fail.v1",
			}
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeTrue())
		})

		It("should propagate non-ErrEntityNotFound errors from FindApplicationID", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockSubscriptionDeps{appErr: dbErr, exposureIDs: map[string]int{}}
			failRepo := eventsubscription.NewRepository(client, cache, failDeps)

			data := &eventsubscription.EventSubscriptionData{
				Meta:            shared.NewMetadata("prod--platform--narvi", "fail-sub", nil),
				StatusPhase:     "UNKNOWN",
				EventType:       "de.telekom.fail.v1",
				DeliveryType:    "CALLBACK",
				OwnerAppName:    "consumer-app",
				OwnerTeamName:   "platform--narvi",
				TargetEventType: "de.telekom.fail.v1",
			}
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should populate the meta cache after upsert", func() {
			data := &eventsubscription.EventSubscriptionData{
				Meta:            shared.NewMetadata("prod--platform--narvi", "cached-sub", nil),
				StatusPhase:     "READY",
				EventType:       "de.telekom.cached.v1",
				DeliveryType:    "CALLBACK",
				OwnerAppName:    "consumer-app",
				OwnerTeamName:   "platform--narvi",
				TargetEventType: "de.telekom.cached.v1",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, found := cache.Get("eventsubscription", "meta:prod--platform--narvi:cached-sub")
			Expect(found).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing event subscription", func() {
			data := &eventsubscription.EventSubscriptionData{
				Meta:            shared.NewMetadata("prod--platform--narvi", "del-sub", nil),
				StatusPhase:     "READY",
				EventType:       "de.telekom.delete.v1",
				DeliveryType:    "CALLBACK",
				OwnerAppName:    "consumer-app",
				OwnerTeamName:   "platform--narvi",
				TargetEventType: "de.telekom.delete.v1",
			}
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := eventsubscription.EventSubscriptionKey{
				EventType:     "de.telekom.delete.v1",
				OwnerAppName:  "consumer-app",
				OwnerTeamName: "platform--narvi",
				Namespace:     "prod--platform--narvi",
				Name:          "del-sub",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			count, err := client.EventSubscription.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should be idempotent for non-existent subscription", func() {
			key := eventsubscription.EventSubscriptionKey{
				EventType:     "de.telekom.nonexistent.v1",
				OwnerAppName:  "consumer-app",
				OwnerTeamName: "platform--narvi",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})
	})
})
