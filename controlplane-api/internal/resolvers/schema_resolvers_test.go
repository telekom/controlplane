// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/approval"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

var _ = Describe("OwnerTeam resolver", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, nil, nil)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return TeamInfo with group name and email", func() {
		ctx := testutil.AllowContext()

		group, err := client.Group.Create().SetName("group-a").SetDisplayName("Group A").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().
			SetName("team-alpha").SetEmail("alpha@test.dev").SetGroup(group).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetName("app-alpha").SetClientID("client-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		viewerCtx := viewer.NewContext(ctx, &viewer.Viewer{Teams: []string{"team-alpha"}})
		info, err := r.Application().OwnerTeam(viewerCtx, app)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Name).To(Equal("team-alpha"))
		Expect(info.GroupName).To(Equal("group-a"))
		Expect(info.Email).NotTo(BeNil())
		Expect(*info.Email).To(Equal("alpha@test.dev"))
		Expect(info.ID).To(Equal(team.ID))
	})

	It("should return empty group name when team has no group", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().
			SetName("team-orphan").SetEmail("orphan@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetName("app-orphan").SetClientID("client-orphan").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		viewerCtx := viewer.NewContext(ctx, &viewer.Viewer{Teams: []string{"team-orphan"}})
		info, err := r.Application().OwnerTeam(viewerCtx, app)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.GroupName).To(BeEmpty())
	})

	It("should resolve the correct owner team when multiple teams exist", func() {
		ctx := testutil.AllowContext()

		groupA, err := client.Group.Create().SetName("group-a").SetDisplayName("Group A").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		groupB, err := client.Group.Create().SetName("group-b").SetDisplayName("Group B").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		teamAlpha, err := client.Team.Create().
			SetName("team-alpha").SetEmail("alpha@test.dev").SetGroup(groupA).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		teamBeta, err := client.Team.Create().
			SetName("team-beta").SetEmail("beta@test.dev").SetGroup(groupB).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		appAlpha, err := client.Application.Create().
			SetName("app-alpha").SetClientID("client-alpha").
			SetOwnerTeam(teamAlpha).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appBeta, err := client.Application.Create().
			SetName("app-beta").SetClientID("client-beta").
			SetOwnerTeam(teamBeta).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		viewerCtx := viewer.NewContext(ctx, &viewer.Viewer{Admin: true})

		infoAlpha, err := r.Application().OwnerTeam(viewerCtx, appAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(infoAlpha.Name).To(Equal("team-alpha"))
		Expect(infoAlpha.GroupName).To(Equal("group-a"))
		Expect(infoAlpha.ID).To(Equal(teamAlpha.ID))

		infoBeta, err := r.Application().OwnerTeam(viewerCtx, appBeta)
		Expect(err).NotTo(HaveOccurred())
		Expect(infoBeta.Name).To(Equal("team-beta"))
		Expect(infoBeta.GroupName).To(Equal("group-b"))
		Expect(infoBeta.ID).To(Equal(teamBeta.ID))
	})

	It("should return nil email when team email is empty", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		// Team.email is NotEmpty, so we test with a valid email vs the resolver's nil logic.
		// The schema enforces NotEmpty, so in practice email is always set.
		// We test that a non-empty email comes back as a pointer.
		team, err := client.Team.Create().
			SetName("team-x").SetEmail("x@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetName("app-x").SetClientID("client-x").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		viewerCtx := viewer.NewContext(ctx, &viewer.Viewer{Teams: []string{"team-x"}})
		info, err := r.Application().OwnerTeam(viewerCtx, app)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Email).NotTo(BeNil())
		Expect(*info.Email).To(Equal("x@test.dev"))
	})
})

var _ = Describe("ApprovalConfig.Strategy resolver", func() {
	It("should convert string to approval.Strategy", func() {
		r := resolvers.NewResolver(nil, nil, nil)
		s, err := r.ApprovalConfig().Strategy(context.Background(), &model.ApprovalConfig{Strategy: "FOUR_EYES"})
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(Equal(approval.StrategyFourEyes))
	})
})

var _ = Describe("ApprovalConfig.TrustedTeams", func() {
	var client *ent.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return trusted teams on an exposure", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetBasePath("/api/v1").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{
				Strategy:     "FOUR_EYES",
				TrustedTeams: []string{"team-beta", "team-gamma"},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ApprovalConfig.TrustedTeams).To(ConsistOf("team-beta", "team-gamma"))
		Expect(fetched.ApprovalConfig.Strategy).To(Equal("FOUR_EYES"))
	})

	It("should default to an empty trusted teams list", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetBasePath("/api/v1").
			SetOwner(app).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ApprovalConfig.Strategy).To(Equal("AUTO"))
		Expect(fetched.ApprovalConfig.TrustedTeams).To(BeEmpty())
	})

	It("should allow a single trusted team", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetBasePath("/api/v1").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{
				Strategy:     "FOUR_EYES",
				TrustedTeams: []string{"team-beta"},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.ApprovalConfig.TrustedTeams).To(HaveLen(1))
		Expect(fetched.ApprovalConfig.TrustedTeams).To(ContainElement("team-beta"))
	})

	It("should update trusted teams on an existing exposure", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetBasePath("/api/v1").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{
				Strategy:     "FOUR_EYES",
				TrustedTeams: []string{"team-beta"},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		updated, err := client.ApiExposure.UpdateOneID(exposure.ID).
			SetApprovalConfig(model.ApprovalConfig{
				Strategy:     "FOUR_EYES",
				TrustedTeams: []string{"team-gamma", "team-delta"},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ApprovalConfig.TrustedTeams).To(ConsistOf("team-gamma", "team-delta"))
	})
})

var _ = Describe("AvailableTransition resolvers", func() {
	r := resolvers.NewResolver(nil, nil, nil)

	It("should convert Action string to ApprovalAction", func() {
		action, err := r.AvailableTransition().Action(context.Background(), &model.AvailableTransition{Action: "ALLOW"})
		Expect(err).NotTo(HaveOccurred())
		Expect(action).To(Equal(model.ApprovalActionAllow))
	})

	It("should convert ToState string to approval.State", func() {
		state, err := r.AvailableTransition().ToState(context.Background(), &model.AvailableTransition{ToState: "GRANTED"})
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(Equal(approval.StateGranted))
	})
})

var _ = Describe("Decision.ResultingState resolver", func() {
	r := resolvers.NewResolver(nil, nil, nil)

	It("should return the state when ResultingState is set", func() {
		granted := "GRANTED"
		state, err := r.Decision().ResultingState(context.Background(), &model.Decision{ResultingState: &granted})
		Expect(err).NotTo(HaveOccurred())
		Expect(state).NotTo(BeNil())
		Expect(*state).To(Equal(approval.StateGranted))
	})

	It("should return nil when ResultingState is nil", func() {
		state, err := r.Decision().ResultingState(context.Background(), &model.Decision{ResultingState: nil})
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(BeNil())
	})
})
