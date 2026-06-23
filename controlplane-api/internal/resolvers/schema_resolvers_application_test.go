// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Application.ExternalIds", func() {
	var client *ent.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return externalIds", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").SetExternalIds([]model.ExternalId{
			{
				Id:     "abc",
				Scheme: "schema1",
			},
			{
				Id:     "123",
				Scheme: "schema2",
			},
		}).
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIds).To(ContainElements(model.ExternalId{
			Id:     "abc",
			Scheme: "schema1",
		}, model.ExternalId{
			Id:     "123",
			Scheme: "schema2",
		}))
	})

	It("should default to an empty externalId list", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIds).To(BeEmpty())
	})

	It("should allow a single externalId team", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").SetExternalIds([]model.ExternalId{
			{
				Id:     "abc",
				Scheme: "schema1",
			},
		}).
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIds).To(HaveLen(1))
		Expect(fetched.ExternalIds).To(ContainElements(model.ExternalId{
			Id:     "abc",
			Scheme: "schema1",
		}))
	})

	It("should update externalIds", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").SetExternalIds([]model.ExternalId{
			{
				Id:     "abc",
				Scheme: "schema1",
			},
		}).
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIds).To(HaveLen(1))
		Expect(fetched.ExternalIds).To(ContainElements(model.ExternalId{
			Id:     "abc",
			Scheme: "schema1",
		}))

		updated, err := client.Application.UpdateOneID(app.ID).AppendExternalIds([]model.ExternalId{
			{
				Id:     "123",
				Scheme: "schema2",
			},
		}).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ExternalIds).To(ContainElements(model.ExternalId{
			Id:     "abc",
			Scheme: "schema1",
		}, model.ExternalId{
			Id:     "123",
			Scheme: "schema2",
		}))

		updated, err = client.Application.UpdateOneID(app.ID).ClearExternalIds().Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ExternalIds).To(BeEmpty())
	})
})

var _ = Describe("Application.IpRestrictions", func() {
	var client *ent.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return IpRestrictions", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetIPRestrictions(model.IpRestrictions{
				Allow: []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
				Deny:  []string{"127.0.0.4", "127.0.0.5", "127.0.0.6"},
			}).
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(ContainElements([]string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}))
		Expect(fetched.IPRestrictions.Allow).To(HaveLen(3))
		Expect(fetched.IPRestrictions.Deny).To(ContainElements([]string{"127.0.0.4", "127.0.0.5", "127.0.0.6"}))
		Expect(fetched.IPRestrictions.Deny).To(HaveLen(3))
	})

	It("should default to an empty IpRestrictions list", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(HaveLen(0))
		Expect(fetched.IPRestrictions.Deny).To(HaveLen(0))
	})

	It("should allow a single IpRestrictions team", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetIPRestrictions(model.IpRestrictions{
				Allow: []string{"127.0.0.1"},
				Deny:  []string{"127.0.0.4"},
			}).
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(ContainElements([]string{"127.0.0.1"}))
		Expect(fetched.IPRestrictions.Allow).To(HaveLen(1))
		Expect(fetched.IPRestrictions.Deny).To(ContainElements([]string{"127.0.0.4"}))
		Expect(fetched.IPRestrictions.Deny).To(HaveLen(1))
	})

	It("should update IpRestrictions", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetIPRestrictions(model.IpRestrictions{
				Allow: []string{"127.0.0.1"},
				Deny:  []string{"127.0.0.4"},
			}).
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(ContainElements([]string{"127.0.0.1"}))
		Expect(fetched.IPRestrictions.Allow).To(HaveLen(1))
		Expect(fetched.IPRestrictions.Deny).To(ContainElements([]string{"127.0.0.4"}))
		Expect(fetched.IPRestrictions.Deny).To(HaveLen(1))

		updated, err := client.Application.UpdateOneID(app.ID).SetIPRestrictions(model.IpRestrictions{
			Allow: []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
			Deny:  []string{"127.0.0.4", "127.0.0.5", "127.0.0.6"},
		}).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.IPRestrictions.Allow).To(ContainElements([]string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}))
		Expect(updated.IPRestrictions.Allow).To(HaveLen(3))
		Expect(updated.IPRestrictions.Deny).To(ContainElements([]string{"127.0.0.4", "127.0.0.5", "127.0.0.6"}))
		Expect(updated.IPRestrictions.Deny).To(HaveLen(3))

		updated, err = client.Application.UpdateOneID(app.ID).ClearIPRestrictions().Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.IPRestrictions.Allow).To(BeEmpty())
		Expect(updated.IPRestrictions.Deny).To(BeEmpty())
	})
})
