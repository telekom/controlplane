// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription_test

import (
	"context"
	"errors"
	"time"

	"entgo.io/ent/privacy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entapiexposure "github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	entapisub "github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"

	"github.com/telekom/controlplane/projector/internal/domain/apisubscription"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// These tests use the real IDResolver and EdgeCache to cover the race where
// the target ApiExposure is projected while the subscription is unresolved.
var _ = Describe("ApiSubscription Repository with real IDResolver", func() {
	const negTTL = 5 * time.Second

	var (
		client     *ent.Client
		cache      *infrastructure.EdgeCache
		repo       *apisubscription.Repository
		ctx        context.Context
		now        time.Time
		providerID int
	)

	data := func() *apisubscription.APISubscriptionData {
		return &apisubscription.APISubscriptionData{
			Meta:           shared.Metadata{Namespace: "prod--platform--narvi", Name: "race-sub", Environment: "prod"},
			StatusPhase:    "READY",
			BasePath:       "/api/v1/race",
			M2MAuthMethod:  "OAUTH2_CLIENT",
			OwnerAppName:   "consumer-app",
			OwnerTeamName:  "platform--narvi",
			TargetBasePath: "/api/v1/race",
		}
	}

	targetID := func() *int {
		sub, err := client.ApiSubscription.Query().
			Where(entapisub.BasePathEQ("/api/v1/race")).
			WithTarget().
			Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		if sub.Edges.Target == nil {
			return nil
		}
		return &sub.Edges.Target.ID
	}

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent-resolver?mode=memory&_fk=1")

		z, err := client.Zone.Create().SetName("caas").SetVisibility(zone.VisibilityEnterprise).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		t, err := client.Team.Create().
			SetName("platform--narvi").SetEmail("narvi@example.com").SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Application.Create().
			SetName("consumer-app").SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		provider, err := client.Application.Create().
			SetName("provider-app").SetNamespace("platform--narvi").
			SetOwnerTeamID(t.ID).SetZoneID(z.ID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		providerID = provider.ID

		now = time.Unix(1_700_000_000, 0)
		resolver := infrastructure.NewIDResolver(client, cache,
			infrastructure.WithNegativeCacheTTL(negTTL),
			infrastructure.WithNowFunc(func() time.Time { return now }),
		)
		repo = apisubscription.NewRepository(client, cache, resolver)
	})

	AfterEach(func() {
		_ = client.Close()
		cache.Close()
	})

	It("keeps signalling a retryable missing dependency until a replay resolves the target", func() {
		// 1. Subscription projected before its target exists.
		err := repo.Upsert(ctx, data())
		Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		Expect(targetID()).To(BeNil())

		// 2. The exposure row appears afterwards (as projected by another
		// worker) and is linked to the subscription.
		exposure, err := client.ApiExposure.Create().
			SetBasePath("/api/v1/race").
			SetNamespace("platform--narvi").
			SetVisibility(entapiexposure.VisibilityWorld).
			SetActive(true).
			SetFeatures([]string{}).
			SetOwnerID(providerID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		// 3. A replay within the negative-cache TTL still cannot resolve the
		// target; it must remain retryable rather than report success.
		now = now.Add(negTTL / 2)
		err = repo.Upsert(ctx, data())
		Expect(errors.Is(err, runtime.ErrDependencyMissing)).To(BeTrue())
		Expect(targetID()).To(BeNil())

		// 4. A retry after the TTL resolves and links the target.
		now = now.Add(negTTL)
		Expect(repo.Upsert(ctx, data())).To(Succeed())
		Expect(targetID()).To(HaveValue(Equal(exposure.ID)))
	})
})
