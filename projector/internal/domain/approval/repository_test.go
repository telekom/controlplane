// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval_test

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
	entapproval "github.com/telekom/controlplane/controlplane-api/ent/approval"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
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
	subIDs map[string]int // key: "namespace:name"
	subErr error          // if non-nil, FindAPISubscriptionByMeta always returns this error
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
			subIDs: map[string]int{"prod--platform--narvi:my-sub": subID},
		}

		repo = approval.NewRepository(client, cache, deps)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	baseData := func() *approval.ApprovalData {
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
			SubscriptionNamespace: "prod--platform--narvi",
			SubscriptionName:      "my-sub",
		}
	}

	Describe("Upsert", func() {
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

			// Verify subscription FK is set.
			Expect(a.Edges.APISubscription).NotTo(BeNil())
			Expect(a.Edges.APISubscription.ID).To(Equal(subID))
		})

		It("should return ErrDependencyMissing when subscription is not cached", func() {
			missingDeps := &mockApprovalDeps{
				subIDs: map[string]int{}, // empty — no subscription found
			}
			repo = approval.NewRepository(client, cache, missingDeps)

			data := baseData()
			err := repo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		})

		It("should propagate non-ErrEntityNotFound errors from FindAPISubscriptionByMeta", func() {
			dbErr := errors.New("connection refused")
			failDeps := &mockApprovalDeps{
				subIDs: map[string]int{},
				subErr: dbErr,
			}
			failRepo := approval.NewRepository(client, cache, failDeps)

			data := baseData()
			err := failRepo.Upsert(ctx, data)
			Expect(err).To(HaveOccurred())
			Expect(runtime.IsDependencyMissing(err)).To(BeFalse())
			Expect(errors.Is(err, dbErr)).To(BeTrue())
		})

		It("should update an existing approval on conflict", func() {
			data := baseData()
			Expect(repo.Upsert(ctx, data)).To(Succeed())

			// Update state.
			data.State = "REJECTED"
			data.StatusPhase = "ERROR"
			data.StatusMessage = "approval rejected"
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
