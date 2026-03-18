// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package interceptor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	entgen "github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/interceptor"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

var _ = Describe("TeamFilterInterceptor", func() {

	var client *entgen.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		client.Intercept(interceptor.TeamFilterInterceptor())
	})

	AfterEach(func() {
		client.Close()
	})

	// viewerCtx creates a context with the given viewer and privacy bypass.
	viewerCtx := func(v *viewer.Viewer) context.Context {
		return viewer.NewContext(testutil.AllowContext(), v)
	}

	seed := func() {
		ctx := testutil.AllowContext()

		// Public reference data.
		zoneEU, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Environment.Create().SetName("env-dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		groupA, err := client.Group.Create().SetName("group-a").SetDisplayName("Group A").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		groupB, err := client.Group.Create().SetName("group-b").SetDisplayName("Group B").Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Teams
		teamAlpha, err := client.Team.Create().
			SetName("team-alpha").SetEmail("alpha@test.dev").SetGroup(groupA).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		teamBeta, err := client.Team.Create().
			SetName("team-beta").SetEmail("beta@test.dev").SetGroup(groupB).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Applications
		appAlpha, err := client.Application.Create().
			SetName("app-alpha").SetClientID("client-alpha").
			SetOwnerTeam(teamAlpha).SetZone(zoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		appBeta, err := client.Application.Create().
			SetName("app-beta").SetClientID("client-beta").
			SetOwnerTeam(teamBeta).SetZone(zoneEU).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		// API Exposures
		exposureAlpha, err := client.ApiExposure.Create().
			SetBasePath("/alpha").SetOwner(appAlpha).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.ApiExposure.Create().
			SetBasePath("/beta").SetOwner(appBeta).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Subscription: app-beta subscribes to exposure-alpha (cross-team).
		sub, err := client.ApiSubscription.Create().
			SetBasePath("/alpha").
			SetOwner(appBeta).
			SetTarget(exposureAlpha).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Approval + ApprovalRequest on that subscription.
		_, err = client.Approval.Create().
			SetAction("ALLOW").
			SetRequester(model.RequesterInfo{TeamName: "team-beta"}).
			SetDecider(model.DeciderInfo{TeamName: "team-alpha"}).
			SetAPISubscription(sub).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.ApprovalRequest.Create().
			SetAction("ALLOW").
			SetRequester(model.RequesterInfo{TeamName: "team-beta"}).
			SetDecider(model.DeciderInfo{TeamName: "team-alpha"}).
			SetAPISubscription(sub).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
	}

	Context("when viewer is nil or empty", func() {
		BeforeEach(func() { seed() })

		It("should pass through without filtering", func() {
			// No viewer in context — interceptor skips, privacy will handle denial.
			// We use AllowContext to bypass privacy so we can observe the pass-through.
			ctx := testutil.AllowContext()
			teams, err := client.Team.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(teams).To(HaveLen(2))
		})

		It("should pass through without filtering", func() {
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{}})
			// Interceptor passes through; privacy would deny in production.
			teams, err := client.Team.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(teams).To(HaveLen(2))
		})
	})

	Context("when viewer is admin", func() {
		BeforeEach(func() { seed() })

		It("should see all teams", func() {
			teams, err := client.Team.Query().All(viewerCtx(&viewer.Viewer{Admin: true}))
			Expect(err).NotTo(HaveOccurred())
			Expect(teams).To(HaveLen(2))
		})

		It("should see all applications", func() {
			apps, err := client.Application.Query().All(viewerCtx(&viewer.Viewer{Admin: true}))
			Expect(err).NotTo(HaveOccurred())
			Expect(apps).To(HaveLen(2))
		})

		It("should see all exposures", func() {
			exps, err := client.ApiExposure.Query().All(viewerCtx(&viewer.Viewer{Admin: true}))
			Expect(err).NotTo(HaveOccurred())
			Expect(exps).To(HaveLen(2))
		})

		It("should see all subscriptions", func() {
			subs, err := client.ApiSubscription.Query().All(viewerCtx(&viewer.Viewer{Admin: true}))
			Expect(err).NotTo(HaveOccurred())
			Expect(subs).To(HaveLen(1))
		})

		It("should see all approvals", func() {
			aprs, err := client.Approval.Query().All(viewerCtx(&viewer.Viewer{Admin: true}))
			Expect(err).NotTo(HaveOccurred())
			Expect(aprs).To(HaveLen(1))
		})

		It("should see all approval requests", func() {
			ars, err := client.ApprovalRequest.Query().All(viewerCtx(&viewer.Viewer{Admin: true}))
			Expect(err).NotTo(HaveOccurred())
			Expect(ars).To(HaveLen(1))
		})
	})

	Context("when viewer belongs to team-alpha", func() {
		BeforeEach(func() { seed() })

		alphaCtx := func() context.Context {
			return viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha"}})
		}

		It("should only see team-alpha", func() {
			teams, err := client.Team.Query().All(alphaCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(teams).To(HaveLen(1))
			Expect(teams[0].Name).To(Equal("team-alpha"))
		})

		It("should only see app-alpha", func() {
			apps, err := client.Application.Query().All(alphaCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(apps).To(HaveLen(1))
			Expect(apps[0].Name).To(Equal("app-alpha"))
		})

		It("should only see exposure-alpha", func() {
			exps, err := client.ApiExposure.Query().All(alphaCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(exps).To(HaveLen(1))
			Expect(exps[0].BasePath).To(Equal("/alpha"))
		})

		It("should not see subscriptions (team-alpha has none)", func() {
			subs, err := client.ApiSubscription.Query().All(alphaCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(subs).To(BeEmpty())
		})

		It("should see approvals where team-alpha is the target provider", func() {
			aprs, err := client.Approval.Query().All(alphaCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(aprs).To(HaveLen(1))
		})

		It("should see approval requests where team-alpha is the target provider", func() {
			ars, err := client.ApprovalRequest.Query().All(alphaCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(ars).To(HaveLen(1))
		})
	})

	Context("when viewer belongs to both teams", func() {
		BeforeEach(func() { seed() })

		bothCtx := func() context.Context {
			return viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha", "team-beta"}})
		}

		It("should see all teams", func() {
			teams, err := client.Team.Query().All(bothCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(teams).To(HaveLen(2))
		})

		It("should see all applications", func() {
			apps, err := client.Application.Query().All(bothCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(apps).To(HaveLen(2))
		})

		It("should see all exposures", func() {
			exps, err := client.ApiExposure.Query().All(bothCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(exps).To(HaveLen(2))
		})

		It("should see the subscription", func() {
			subs, err := client.ApiSubscription.Query().All(bothCtx())
			Expect(err).NotTo(HaveOccurred())
			Expect(subs).To(HaveLen(1))
		})
	})

	Context("public entities (no team filtering)", func() {
		BeforeEach(func() { seed() })

		It("should not filter zones", func() {
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha"}})
			zones, err := client.Zone.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(zones).To(HaveLen(1))
		})

		It("should not filter groups", func() {
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha"}})
			groups, err := client.Group.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(HaveLen(2))
		})

		It("should not filter environments", func() {
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha"}})
			envs, err := client.Environment.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(envs).To(HaveLen(1))
		})
	})

	Context("when an unsupported query type is encountered", func() {
		It("should return an error", func() {
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha"}})

			i := interceptor.TeamFilterInterceptor()
			// Traverse wraps the interceptor around a no-op querier so we can invoke it directly.
			querier := i.Intercept(entgen.QuerierFunc(func(_ context.Context, _ entgen.Query) (entgen.Value, error) {
				return nil, nil
			}))
			_, err := querier.Query(ctx, "unsupported-query-type")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported query type"))
		})
	})
})
