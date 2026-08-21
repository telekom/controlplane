// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Catalogue owner application resolvers", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
	})

	AfterEach(func() {
		client.Close()
	})

	It("resolves Api.ownerApplication with applicationId and ictoNumber", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().
			SetNamespace("default").SetName("team-alpha").SetEmail("alpha@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").
			SetName("app-alpha").
			SetClientID("client-alpha").
			SetExternalIds([]model.ExternalId{{Id: "ICTO-12345", Scheme: "ICTO"}}).
			SetOwnerTeam(team).
			SetZone(zone).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		apiObj, err := client.Api.Create().
			SetNamespace("default").
			SetBasePath("/orders").
			SetVersion("v1").
			SetOwner(team).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/orders").
			SetOwner(app).
			SetAPI(apiObj).
			SetActive(true).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		ownerApp, err := r.Api().OwnerApplication(ctx, apiObj)
		Expect(err).NotTo(HaveOccurred())
		Expect(ownerApp).NotTo(BeNil())
		Expect(ownerApp.ApplicationID).To(Equal(app.ID))
		Expect(ownerApp.IctoNumber).NotTo(BeNil())
		Expect(*ownerApp.IctoNumber).To(Equal("ICTO-12345"))
	})

	It("resolves EventType.ownerApplication with applicationId and ictoNumber", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().
			SetNamespace("default").SetName("team-alpha").SetEmail("alpha@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").
			SetName("app-alpha").
			SetClientID("client-alpha").
			SetExternalIds([]model.ExternalId{{Id: "123456", Scheme: "icto"}}).
			SetOwnerTeam(team).
			SetZone(zone).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		eventType, err := client.EventType.Create().
			SetNamespace("default").
			SetEventType("order.created").
			SetVersion("v1").
			SetOwner(team).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.EventExposure.Create().
			SetNamespace("default").
			SetEventType("order.created").
			SetOwner(app).
			SetEventTypeDef(eventType).
			SetActive(true).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		ownerApp, err := r.EventType().OwnerApplication(ctx, eventType)
		Expect(err).NotTo(HaveOccurred())
		Expect(ownerApp).NotTo(BeNil())
		Expect(ownerApp.ApplicationID).To(Equal(app.ID))
		Expect(ownerApp.IctoNumber).NotTo(BeNil())
		Expect(*ownerApp.IctoNumber).To(Equal("123456"))
	})

	It("returns nil ictoNumber when no icto external ID exists", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().
			SetNamespace("default").SetName("team-alpha").SetEmail("alpha@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").
			SetName("app-alpha").
			SetClientID("client-alpha").
			SetExternalIds([]model.ExternalId{{Id: "A-42", Scheme: "sap"}}).
			SetOwnerTeam(team).
			SetZone(zone).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		apiObj, err := client.Api.Create().
			SetNamespace("default").
			SetBasePath("/orders").
			SetVersion("v1").
			SetOwner(team).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/orders").
			SetOwner(app).
			SetAPI(apiObj).
			SetActive(true).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		ownerApp, err := r.Api().OwnerApplication(ctx, apiObj)
		Expect(err).NotTo(HaveOccurred())
		Expect(ownerApp).NotTo(BeNil())
		Expect(ownerApp.ApplicationID).To(Equal(app.ID))
		Expect(ownerApp.IctoNumber).To(BeNil())
	})
})
