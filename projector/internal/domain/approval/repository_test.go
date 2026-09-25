// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/mattn/go-sqlite3"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entapiexposure "github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	entapplication "github.com/telekom/controlplane/controlplane-api/ent/application"
	entapproval "github.com/telekom/controlplane/controlplane-api/ent/approval"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/controlplane-api/ent/eventsubscription"
	entlistener "github.com/telekom/controlplane/controlplane-api/ent/listener"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	"github.com/telekom/controlplane/projector/internal/domain/approval"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// mockApprovalDeps implements approval.ApprovalDeps for testing.
type mockApprovalDeps struct {
	subIDs        map[string]int // key: "namespace:name"
	subErr        error          // if non-nil, FindAPISubscriptionByMeta always returns this error
	eventSubIDs   map[string]int // key: "namespace:name"
	eventSubErr   error          // if non-nil, FindEventSubscriptionByMeta always returns this error
	agenticSubIDs map[string]int // key: "namespace:name"
	agenticSubErr error          // if non-nil, FindAgenticSubscriptionByMeta always returns this error
	evicted       []string       // tracks eviction calls as "namespace:name"
}

func (m *mockApprovalDeps) FindAPISubscriptionByMeta(_ context.Context, namespace, name string) (int, error) {
	if m.subErr != nil {
		return 0, m.subErr
	}
	key := namespace + ":" + name
	if id, ok := m.subIDs[key]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("api_subscription %s/%s: %w", namespace, name, infrastructure.ErrEntityNotFound)
}

func (m *mockApprovalDeps) FindEventSubscriptionByMeta(_ context.Context, namespace, name string) (int, error) {
	if m.eventSubErr != nil {
		return 0, m.eventSubErr
	}
	key := namespace + ":" + name
	if id, ok := m.eventSubIDs[key]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("event_subscription %s/%s: %w", namespace, name, infrastructure.ErrEntityNotFound)
}

func (m *mockApprovalDeps) FindAgenticSubscriptionByMeta(_ context.Context, namespace, name string) (int, error) {
	if m.agenticSubErr != nil {
		return 0, m.agenticSubErr
	}
	key := namespace + ":" + name
	if id, ok := m.agenticSubIDs[key]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("agentic_subscription %s/%s: %w", namespace, name, infrastructure.ErrEntityNotFound)
}

func (m *mockApprovalDeps) EvictAPISubscription(namespace, name string) {
	m.evicted = append(m.evicted, namespace+":"+name)
}

func (m *mockApprovalDeps) EvictEventSubscription(namespace, name string) {
	m.evicted = append(m.evicted, namespace+":"+name)
}

func (m *mockApprovalDeps) EvictAgenticSubscription(namespace, name string) {
	m.evicted = append(m.evicted, namespace+":"+name)
}

var _ = Describe("Approval Repository", func() {
	var (
		client *ent.Client
		cache  *infrastructure.EdgeCache
		deps   *mockApprovalDeps
		repo   *approval.Repository
		ctx    context.Context
		subID  int
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")

		// Seed Zone -> Team -> Application -> ApiSubscription dependency chain.
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

		// Seed a target ApiExposure + ApiSubscription.
		providerApp, err := client.Application.Create().
			SetName("provider-app").
			SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).
			SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.ApiExposure.Create().
			SetBasePath("/api/v1/users").
			SetNamespace("platform--narvi").
			SetVisibility(entapiexposure.VisibilityWorld).
			SetActive(true).
			SetFeatures([]string{}).
			SetOwnerID(providerApp.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.ApiSubscription.Create().
			SetBasePath("/api/v1/users").
			SetNamespace("platform--narvi").
			SetName("my-sub").
			SetOwnerID(app.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		subID = sub.ID

		deps = &mockApprovalDeps{
			subIDs:      map[string]int{"prod--platform--narvi:my-sub": subID},
			eventSubIDs: map[string]int{},
		}

		repo = approval.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	baseData := func() *approval.ApprovalData {
		expiresAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
		return &approval.ApprovalData{
			Meta: shared.Metadata{
				Namespace:   "prod--platform--narvi",
				Name:        "apisubscription--my-sub",
				Environment: "prod",
			},
			StatusPhase:   "READY",
			StatusMessage: "approval granted",
			State:         "GRANTED",
			Action:        "subscribe",
			Strategy:      "FOUR_EYES",
			Requester: model.RequesterInfo{
				TeamName:  "narvi",
				TeamEmail: "narvi@example.com",
			},
			Decider: model.DeciderInfo{
				TeamName: "provider-team",
			},
			Decisions:             []model.Decision{},
			AvailableTransitions:  []model.AvailableTransition{},
			TargetKind:            "ApiSubscription",
			SubscriptionNamespace: "prod--platform--narvi",
			SubscriptionName:      "my-sub",
			ExpiresAt:             &expiresAt,
		}
	}

	Describe("Upsert", func() {
		It("attaches scoped provider and consumer approvals to separate Listener edges", func() {
			app, err := client.Application.Query().Where(entapplication.NameEQ("consumer-app")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			exposure, err := client.ApiExposure.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			makeListener := func(name string) int {
				l, err := client.Listener.Create().SetName(name).SetNamespace("ns").SetAPIBasePath("/api/v1/users").
					SetApplicationID(app.ID).SetSubscriptionID(subID).SetExposureID(exposure.ID).Save(ctx)
				Expect(err).NotTo(HaveOccurred())
				return l.ID
			}
			first := makeListener("first")
			second := makeListener("second")
			data := baseData()
			data.TargetKind = approval.TargetKindListener
			data.SubscriptionNamespace = "ns"
			data.SubscriptionName = "first"
			data.Meta.Name = "provider-approval"
			data.ApprovalKey = "provider"
			data.Action = "listen-provider"
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			provider, err := client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(provider.QueryListener().OnlyID(ctx)).To(Equal(first))
			data.Meta.Name = "consumer-approval"
			data.ApprovalKey = "consumer"
			data.Action = "listen-consumer"
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			consumer, err := client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(consumer.QueryConsumerListener().OnlyID(ctx)).To(Equal(first))
			data.SubscriptionName = "second"
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			consumer, err = client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(consumer.QueryConsumerListener().OnlyID(ctx)).To(Equal(second))
			count, err := client.Listener.Query().Where(entlistener.NameEQ("first")).QueryConsumerApproval().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
			data.ApprovalKey = "provider"
			data.Action = "listen-provider"
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			consumer, err = client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(consumer.QueryListener().OnlyID(ctx)).To(Equal(second))
			Expect(consumer.QueryConsumerListener().Exist(ctx)).To(BeFalse())
			data.TargetKind = approval.TargetKindAPISubscription
			data.Action = "subscribe"
			data.SubscriptionNamespace = "prod--platform--narvi"
			data.SubscriptionName = "my-sub"
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			consumer, err = client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(consumer.QueryAPISubscription().OnlyID(ctx)).To(Equal(subID))
			Expect(consumer.QueryConsumerListener().Exist(ctx)).To(BeFalse())
			Expect(consumer.QueryListener().Exist(ctx)).To(BeFalse())

			agenticSub, err := client.AgenticSubscription.Create().SetBasePath("/agentic/v1/my-agent").
				SetNamespace("ns").SetName("agentic-sub").SetOwnerID(app.ID).Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			deps.agenticSubIDs = map[string]int{"ns:agentic-sub": agenticSub.ID}
			data.SubscriptionNamespace = "ns"
			data.TargetKind = approval.TargetKindAgenticSubscription
			data.SubscriptionName = "agentic-sub"
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			for _, gate := range []string{"provider", "consumer"} {
				data.TargetKind = approval.TargetKindListener
				data.SubscriptionName = "second"
				data.ApprovalKey = gate
				data.Action = "listen-" + gate
				Expect(repo.Upsert(ctx, data)).To(Succeed())
				consumer, err = client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(consumer.QueryAgenticSubscription().Exist(ctx)).To(BeFalse())
				Expect(consumer.QueryAPISubscription().Exist(ctx)).To(BeFalse())
				if gate == "provider" {
					Expect(consumer.QueryListener().OnlyID(ctx)).To(Equal(second))
				} else {
					Expect(consumer.QueryConsumerListener().OnlyID(ctx)).To(Equal(second))
				}
				data.TargetKind = approval.TargetKindAgenticSubscription
				data.SubscriptionName = "agentic-sub"
				data.Action = "subscribe"
				Expect(repo.Upsert(ctx, data)).To(Succeed())
				consumer, err = client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(consumer.QueryAgenticSubscription().OnlyID(ctx)).To(Equal(agenticSub.ID))
				Expect(consumer.QueryListener().Exist(ctx)).To(BeFalse())
				Expect(consumer.QueryConsumerListener().Exist(ctx)).To(BeFalse())
			}
		})

		It("retries missing Listener and refuses unscoped approvals", func() {
			data := baseData()
			data.TargetKind = approval.TargetKindListener
			data.ApprovalKey = "provider"
			data.Action = "listen-provider"
			data.SubscriptionNamespace = "ns"
			data.SubscriptionName = "absent"
			Expect(errors.Is(repo.Upsert(ctx, data), runtime.ErrDependencyMissing)).To(BeTrue())
			data.ApprovalKey = ""
			Expect(repo.Upsert(ctx, data)).To(MatchError(ContainSubstring("invalid Listener approvalKey")))
		})
		It("rejects a second approval for the same Listener gate without changing either row", func() {
			app, err := client.Application.Query().Where(entapplication.NameEQ("consumer-app")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			exposure, err := client.ApiExposure.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			listener, err := client.Listener.Create().SetName("gate").SetNamespace("ns").SetAPIBasePath("/api/v1/users").
				SetApplicationID(app.ID).SetSubscriptionID(subID).SetExposureID(exposure.ID).Save(ctx)
			Expect(err).NotTo(HaveOccurred())
			data := baseData()
			data.TargetKind = approval.TargetKindListener
			data.ApprovalKey = "provider"
			data.Action = "listen-provider"
			data.SubscriptionNamespace = "ns"
			data.SubscriptionName = "gate"
			data.Meta.Name = "first-provider"
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			data.Meta.Name = "second-provider"
			Expect(repo.Upsert(ctx, data)).To(MatchError(ContainSubstring("already belongs to")))
			count, err := client.Approval.Query().Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))
			Expect(listener.QueryProviderApproval().OnlyID(ctx)).To(BeNumerically(">", 0))
			owner, err := listener.QueryProviderApproval().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(owner.Name).To(Equal("first-provider"))
			cache.Wait()
			_, ok := cache.Get("approval", data.Meta.Namespace+":second-provider")
			Expect(ok).To(BeFalse())
		})
		It("should create a new approval with subscription FK", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify the approval was created.
			a, err := client.Approval.Query().
				Where(
					entapproval.NamespaceEQ("prod--platform--narvi"),
					entapproval.NameEQ("apisubscription--my-sub"),
				).
				WithAPISubscription().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.Name).To(Equal("apisubscription--my-sub"))
			Expect(a.Action).To(Equal("subscribe"))
			Expect(a.Strategy.String()).To(Equal("FOUR_EYES"))
			Expect(a.State.String()).To(Equal("GRANTED"))
			Expect(a.StatusPhase.String()).To(Equal("READY"))
			Expect(*a.StatusMessage).To(Equal("approval granted"))
			Expect(a.ExpiresAt).NotTo(BeNil())
			Expect(*a.ExpiresAt).To(BeTemporally("~", time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), time.Second))

			// Verify subscription FK is set.
			Expect(a.Edges.APISubscription).NotTo(BeNil())
			Expect(a.Edges.APISubscription.ID).To(Equal(subID))
		})

		It("should create a new approval with event subscription FK", func() {
			// Seed an EventSubscription.
			z, err := client.Zone.Query().Where(zone.NameEQ("caas")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			t, err := client.Team.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			subscriberApp, err := client.Application.Create().
				SetName("event-consumer").
				SetNamespace("platform--narvi").
				SetOwnerTeamID(t.ID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			eventSub, err := client.EventSubscription.Create().
				SetEventType("user.created").
				SetNamespace("platform--narvi").
				SetName("my-event-sub").
				SetDeliveryType(eventsubscription.DeliveryTypeCallback).
				SetOwnerID(subscriberApp.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			deps.eventSubIDs["prod--platform--narvi:my-event-sub"] = eventSub.ID

			data := baseData()
			data.Meta.Name = "eventsubscription--my-event-sub"
			data.TargetKind = "EventSubscription"
			data.SubscriptionNamespace = "prod--platform--narvi"
			data.SubscriptionName = "my-event-sub"

			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify the approval was created with event subscription FK.
			a, err := client.Approval.Query().
				Where(
					entapproval.NamespaceEQ("prod--platform--narvi"),
					entapproval.NameEQ("eventsubscription--my-event-sub"),
				).
				WithEventSubscription().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.Edges.EventSubscription).NotTo(BeNil())
			Expect(a.Edges.EventSubscription.ID).To(Equal(eventSub.ID))
		})

		It("should create a new approval with agentic subscription FK", func() {
			// Seed an AgenticSubscription.
			z, err := client.Zone.Query().Where(zone.NameEQ("caas")).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			t, err := client.Team.Query().Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			agenticConsumerApp, err := client.Application.Create().
				SetName("agentic-consumer").
				SetNamespace("platform--narvi").
				SetOwnerTeamID(t.ID).
				SetZoneID(z.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			agenticSub, err := client.AgenticSubscription.Create().
				SetBasePath("/agentic/v1/my-agent").
				SetNamespace("platform--narvi").
				SetName("my-agentic-sub").
				SetOwnerID(agenticConsumerApp.ID).
				Save(ctx)
			Expect(err).NotTo(HaveOccurred())

			deps.agenticSubIDs = map[string]int{"prod--platform--narvi:my-agentic-sub": agenticSub.ID}

			data := baseData()
			data.Meta.Name = "agenticsubscription--my-agentic-sub"
			data.TargetKind = "AgenticSubscription"
			data.SubscriptionNamespace = "prod--platform--narvi"
			data.SubscriptionName = "my-agentic-sub"

			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Verify the approval was created with agentic subscription FK.
			a, err := client.Approval.Query().
				Where(
					entapproval.NamespaceEQ("prod--platform--narvi"),
					entapproval.NameEQ("agenticsubscription--my-agentic-sub"),
				).
				WithAgenticSubscription().
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.Edges.AgenticSubscription).NotTo(BeNil())
			Expect(a.Edges.AgenticSubscription.ID).To(Equal(agenticSub.ID))
		})

		It("should return ErrDependencyMissing when subscription is not cached", func() {
			missingDeps := &mockApprovalDeps{
				subIDs:      map[string]int{}, // empty — no subscription found
				eventSubIDs: map[string]int{},
			}
			repo = approval.NewRepository(client, cache, missingDeps)

			data := baseData()
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("should return ErrDependencyMissing when event subscription is not cached", func() {
			data := baseData()
			data.TargetKind = "EventSubscription"
			data.SubscriptionName = "missing-event-sub"
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("should return ErrDependencyMissing when agentic subscription is not cached", func() {
			data := baseData()
			data.TargetKind = "AgenticSubscription"
			data.SubscriptionName = "missing-agentic-sub"
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("should propagate non-ErrEntityNotFound errors from FindAPISubscriptionByMeta", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockApprovalDeps{
				subIDs:      map[string]int{},
				subErr:      dbErr,
				eventSubIDs: map[string]int{},
			}
			failRepo := approval.NewRepository(client, cache, failDeps)

			data := baseData()
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should propagate non-ErrEntityNotFound errors from FindEventSubscriptionByMeta", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockApprovalDeps{
				subIDs:      map[string]int{},
				eventSubIDs: map[string]int{},
				eventSubErr: dbErr,
			}
			failRepo := approval.NewRepository(client, cache, failDeps)

			data := baseData()
			data.TargetKind = "EventSubscription"
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should update an existing approval on conflict", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update state and ExpiresAt.
			data.State = "REJECTED"
			data.StatusPhase = "ERROR"
			data.StatusMessage = "approval rejected"
			updatedExpiresAt := time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC)
			data.ExpiresAt = &updatedExpiresAt
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			a, err := client.Approval.Query().
				Where(
					entapproval.NamespaceEQ("prod--platform--narvi"),
					entapproval.NameEQ("apisubscription--my-sub"),
				).
				Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.State.String()).To(Equal("REJECTED"))
			Expect(a.StatusPhase.String()).To(Equal("ERROR"))
			Expect(*a.StatusMessage).To(Equal("approval rejected"))
			Expect(a.ExpiresAt).NotTo(BeNil())
			Expect(*a.ExpiresAt).To(BeTemporally("~", time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC), time.Second))
		})

		It("rolls back changed fields when the edge update fails", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			data.State = "REJECTED"
			deps.subIDs["prod--platform--narvi:my-sub"] = subID + 10000
			Expect(repo.Upsert(ctx, data)).To(HaveOccurred())
			a, err := client.Approval.Query().Where(entapproval.NameEQ(data.Meta.Name)).Only(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.State.String()).To(Equal("GRANTED"))
			Expect(a.QueryAPISubscription().OnlyID(ctx)).To(Equal(subID))
		})

		It("should maintain cache entry", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			cache.Wait()

			id, ok := cache.Get("approval", "prod--platform--narvi:apisubscription--my-sub")
			Expect(ok).To(BeTrue())
			Expect(id).To(BeNumerically(">", 0))
		})

		It("should only have one row after two upserts", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			count, err := client.Approval.Query().
				Where(
					entapproval.NamespaceEQ("prod--platform--narvi"),
					entapproval.NameEQ("apisubscription--my-sub"),
				).
				Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))
		})
	})

	Describe("Delete", func() {
		It("should delete an existing approval and clean cache", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			key := approval.ApprovalKey{
				Namespace:             "prod--platform--narvi",
				Name:                  "apisubscription--my-sub",
				SubscriptionNamespace: "prod--platform--narvi",
				SubscriptionName:      "my-sub",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())

			// Verify deleted from DB.
			count, err := client.Approval.Query().
				Where(
					entapproval.NamespaceEQ("prod--platform--narvi"),
					entapproval.NameEQ("apisubscription--my-sub"),
				).
				Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))

			// Verify cache cleaned.
			_, ok := cache.Get("approval", "prod--platform--narvi:apisubscription--my-sub")
			Expect(ok).To(BeFalse())
		})

		It("should be idempotent -- deleting a non-existent approval succeeds", func() {
			key := approval.ApprovalKey{
				Namespace: "ns",
				Name:      "nonexistent",
			}
			Expect(repo.Delete(ctx, key)).To(Succeed())
		})
	})
})
