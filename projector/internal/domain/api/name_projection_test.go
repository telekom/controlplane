// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"

	"entgo.io/ent/privacy"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entapi "github.com/telekom/controlplane/controlplane-api/ent/api"
	"github.com/telekom/controlplane/controlplane-api/ent/enttest"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/projector/internal/domain/api"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
)

var _ = Describe("Api name projection", func() {
	var (
		ctx    context.Context
		client *ent.Client
		cache  *infrastructure.EdgeCache
		repo   *api.Repository
		teamID int
		obj    *apiv1.Api
	)

	BeforeEach(func() {
		ctx = privacy.DecisionContext(context.Background(), privacy.Allow)
		var err error
		cache, err = infrastructure.NewEdgeCache(100_000, 10<<20, 64)
		Expect(err).NotTo(HaveOccurred())
		client = enttest.Open(GinkgoT(), "sqlite3", "file:ent?mode=memory&_fk=1")
		owner, err := client.Team.Create().
			SetName("platform--narvi").
			SetEmail("narvi@example.com").
			SetNamespace("platform--narvi").
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		teamID = owner.ID
		repo = api.NewRepository(client, cache, infrastructure.NewIDResolver(client, cache))
		obj = &apiv1.Api{
			ObjectMeta: metav1.ObjectMeta{Name: "users-api", Namespace: "prod--platform--narvi"},
			Spec: apiv1.ApiSpec{
				BasePath: "/api/v1/users",
				Version:  "v1",
			},
		}
	})

	AfterEach(func() {
		Expect(client.Close()).To(Succeed())
		cache.Close()
	})

	project := func() {
		data, err := (&api.Translator{}).Translate(ctx, obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.Upsert(ctx, data)).To(Succeed())
	}

	It("persists the CR name on initial projection", func() {
		project()

		stored, err := client.Api.Query().Where(entapi.BasePathEQ(obj.Spec.BasePath)).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Name).NotTo(BeNil())
		Expect(*stored.Name).To(Equal("users-api"))
	})

	It("updates the name of the existing Api on upsert", func() {
		project()
		original, err := client.Api.Query().Where(entapi.BasePathEQ(obj.Spec.BasePath)).Only(ctx)
		Expect(err).NotTo(HaveOccurred())

		obj.Name = "renamed-users-api"
		project()

		stored, err := client.Api.Query().Where(entapi.BasePathEQ(obj.Spec.BasePath)).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.ID).To(Equal(original.ID))
		Expect(stored.Name).NotTo(BeNil())
		Expect(*stored.Name).To(Equal("renamed-users-api"))
	})

	It("backfills the name of an existing Api with a NULL name", func() {
		legacy, err := client.Api.Create().
			SetBasePath(obj.Spec.BasePath).
			SetVersion(obj.Spec.Version).
			SetNamespace(obj.Namespace).
			SetOwnerID(teamID).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(legacy.Name).To(BeNil())

		project()

		stored, err := client.Api.Query().Where(entapi.BasePathEQ(obj.Spec.BasePath)).Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.ID).To(Equal(legacy.ID))
		Expect(stored.Name).NotTo(BeNil())
		Expect(*stored.Name).To(Equal("users-api"))
	})
})
