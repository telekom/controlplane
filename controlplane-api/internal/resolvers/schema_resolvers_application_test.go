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

var _ = Describe("Application.ExternalIDs", func() {
	var client *ent.Client
	var s *testutil.SeedData

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return externalIDs", func() {
		ctx := testutil.AllowContext()

		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-ext").SetClientID("cid-ext").SetExternalIDs([]model.ExternalID{
			{
				ID:     "abc",
				Scheme: "schema1",
			},
			{
				ID:     "123",
				Scheme: "schema2",
			},
		}).
			SetOwnerTeam(s.TeamAlpha).SetZone(s.ZoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIDs).To(ContainElements(model.ExternalID{
			ID:     "abc",
			Scheme: "schema1",
		}, model.ExternalID{
			ID:     "123",
			Scheme: "schema2",
		}))
	})

	It("should default to an empty externalID list", func() {
		ctx := testutil.AllowContext()

		fetched, err := client.Application.Get(ctx, s.AppAlpha.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIDs).To(BeEmpty())
	})

	It("should allow a single externalID team", func() {
		ctx := testutil.AllowContext()

		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-ext").SetClientID("cid-ext").SetExternalIDs([]model.ExternalID{
			{
				ID:     "abc",
				Scheme: "schema1",
			},
		}).
			SetOwnerTeam(s.TeamAlpha).SetZone(s.ZoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIDs).To(HaveLen(1))
		Expect(fetched.ExternalIDs).To(ContainElements(model.ExternalID{
			ID:     "abc",
			Scheme: "schema1",
		}))
	})

	It("should update externalIDs", func() {
		ctx := testutil.AllowContext()

		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-ext").SetClientID("cid-ext").SetExternalIDs([]model.ExternalID{
			{
				ID:     "abc",
				Scheme: "schema1",
			},
		}).
			SetOwnerTeam(s.TeamAlpha).SetZone(s.ZoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ExternalIDs).To(HaveLen(1))
		Expect(fetched.ExternalIDs).To(ContainElements(model.ExternalID{
			ID:     "abc",
			Scheme: "schema1",
		}))

		updated, err := client.Application.UpdateOneID(app.ID).AppendExternalIDs([]model.ExternalID{
			{
				ID:     "123",
				Scheme: "schema2",
			},
		}).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ExternalIDs).To(ContainElements(model.ExternalID{
			ID:     "abc",
			Scheme: "schema1",
		}, model.ExternalID{
			ID:     "123",
			Scheme: "schema2",
		}))

		updated, err = client.Application.UpdateOneID(app.ID).ClearExternalIDs().Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ExternalIDs).To(BeEmpty())
	})
})

var _ = Describe("Application.IPRestrictions", func() {
	var client *ent.Client
	var s *testutil.SeedData

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return IPRestrictions", func() {
		ctx := testutil.AllowContext()

		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-ip").SetClientID("cid-ip").
			SetIPRestrictions(model.IPRestrictions{
				Allow: []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
				Deny:  []string{"127.0.0.4", "127.0.0.5", "127.0.0.6"},
			}).
			SetOwnerTeam(s.TeamAlpha).SetZone(s.ZoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(ContainElements([]string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}))
		Expect(fetched.IPRestrictions.Allow).To(HaveLen(3))
		Expect(fetched.IPRestrictions.Deny).To(ContainElements([]string{"127.0.0.4", "127.0.0.5", "127.0.0.6"}))
		Expect(fetched.IPRestrictions.Deny).To(HaveLen(3))
	})

	It("should default to an empty IPRestrictions list", func() {
		ctx := testutil.AllowContext()

		fetched, err := client.Application.Get(ctx, s.AppAlpha.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(BeEmpty())
		Expect(fetched.IPRestrictions.Deny).To(BeEmpty())
	})

	It("should allow a single IPRestrictions team", func() {
		ctx := testutil.AllowContext()

		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-ip").SetClientID("cid-ip").
			SetIPRestrictions(model.IPRestrictions{
				Allow: []string{"127.0.0.1"},
				Deny:  []string{"127.0.0.4"},
			}).
			SetOwnerTeam(s.TeamAlpha).SetZone(s.ZoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(ContainElements([]string{"127.0.0.1"}))
		Expect(fetched.IPRestrictions.Allow).To(HaveLen(1))
		Expect(fetched.IPRestrictions.Deny).To(ContainElements([]string{"127.0.0.4"}))
		Expect(fetched.IPRestrictions.Deny).To(HaveLen(1))
	})

	It("should update IPRestrictions", func() {
		ctx := testutil.AllowContext()

		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-ip").SetClientID("cid-ip").
			SetIPRestrictions(model.IPRestrictions{
				Allow: []string{"127.0.0.1"},
				Deny:  []string{"127.0.0.4"},
			}).
			SetOwnerTeam(s.TeamAlpha).SetZone(s.ZoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.Application.Get(ctx, app.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.IPRestrictions.Allow).To(ContainElements([]string{"127.0.0.1"}))
		Expect(fetched.IPRestrictions.Allow).To(HaveLen(1))
		Expect(fetched.IPRestrictions.Deny).To(ContainElements([]string{"127.0.0.4"}))
		Expect(fetched.IPRestrictions.Deny).To(HaveLen(1))

		updated, err := client.Application.UpdateOneID(app.ID).SetIPRestrictions(model.IPRestrictions{
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
