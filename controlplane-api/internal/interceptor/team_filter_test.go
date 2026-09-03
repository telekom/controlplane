// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package interceptor_test

import (
	"context"

	entgen "github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/interceptor"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	// viewerCtx creates a context with the given viewer (no privacy bypass,
	// so the team-filter interceptor is exercised).
	viewerCtx := func(v *viewer.Viewer) context.Context {
		return viewer.NewContext(context.Background(), v)
	}

	seed := func() {
		testutil.SeedStandard(client)
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
			// AllowContext bypasses privacy (which would deny empty teams in production).
			ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{}})
			teams, err := client.Team.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(teams).To(HaveLen(2))
		})
	})

	Context("when viewer is admin", func() {
		BeforeEach(func() { seed() })

		adminCtx := func() context.Context {
			return viewerCtx(&viewer.Viewer{Admin: true})
		}

		DescribeTable("should see all entities",
			func(queryAll func(context.Context) (int, error), expectedLen int) {
				count, err := queryAll(adminCtx())
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(expectedLen))
			},
			Entry("teams", func(ctx context.Context) (int, error) {
				r, e := client.Team.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("applications", func(ctx context.Context) (int, error) {
				r, e := client.Application.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("exposures", func(ctx context.Context) (int, error) {
				r, e := client.ApiExposure.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("subscriptions", func(ctx context.Context) (int, error) {
				r, e := client.ApiSubscription.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("approvals", func(ctx context.Context) (int, error) {
				r, e := client.Approval.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("approval requests", func(ctx context.Context) (int, error) {
				r, e := client.ApprovalRequest.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("members", func(ctx context.Context) (int, error) {
				r, e := client.Member.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("event exposures", func(ctx context.Context) (int, error) {
				r, e := client.EventExposure.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("event subscriptions", func(ctx context.Context) (int, error) {
				r, e := client.EventSubscription.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("permission sets", func(ctx context.Context) (int, error) {
				r, e := client.PermissionSet.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("mcp servers", func(ctx context.Context) (int, error) {
				r, e := client.McpServer.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("agentic exposures", func(ctx context.Context) (int, error) {
				r, e := client.AgenticExposure.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("agentic subscriptions", func(ctx context.Context) (int, error) {
				r, e := client.AgenticSubscription.Query().All(ctx)
				return len(r), e
			}, 1),
		)
	})

	Context("when viewer belongs to team-alpha", func() {
		BeforeEach(func() { seed() })

		alphaCtx := func() context.Context {
			return viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha"}})
		}

		DescribeTable("should only see team-alpha's entities",
			func(queryAll func(context.Context) (int, error), expectedLen int) {
				count, err := queryAll(alphaCtx())
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(expectedLen))
			},
			Entry("teams", func(ctx context.Context) (int, error) {
				r, e := client.Team.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("applications", func(ctx context.Context) (int, error) {
				r, e := client.Application.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("exposures", func(ctx context.Context) (int, error) {
				r, e := client.ApiExposure.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("subscriptions (team-alpha has none)", func(ctx context.Context) (int, error) {
				r, e := client.ApiSubscription.Query().All(ctx)
				return len(r), e
			}, 0),
			Entry("approvals (team-alpha is target provider)", func(ctx context.Context) (int, error) {
				r, e := client.Approval.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("approval requests (team-alpha is target provider)", func(ctx context.Context) (int, error) {
				r, e := client.ApprovalRequest.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("members", func(ctx context.Context) (int, error) {
				r, e := client.Member.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("event exposures (team-alpha owns one)", func(ctx context.Context) (int, error) {
				r, e := client.EventExposure.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("event subscriptions (team-alpha has none)", func(ctx context.Context) (int, error) {
				r, e := client.EventSubscription.Query().All(ctx)
				return len(r), e
			}, 0),
			Entry("permission sets (team-alpha owns one)", func(ctx context.Context) (int, error) {
				r, e := client.PermissionSet.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("agentic exposures (team-alpha owns one)", func(ctx context.Context) (int, error) {
				r, e := client.AgenticExposure.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("agentic subscriptions (team-alpha has none)", func(ctx context.Context) (int, error) {
				r, e := client.AgenticSubscription.Query().All(ctx)
				return len(r), e
			}, 0),
		)

		// PermissionSet exposes access-control data, so beyond the row-count
		// checks above, verify the actual returned row and owner-team to
		// guard against leakage of another team's permissions.
		It("should return only team-alpha's own permission set, scoped correctly", func() {
			ctx := alphaCtx()
			result, err := client.PermissionSet.Query().
				WithOwnerApplication(func(q *entgen.ApplicationQuery) {
					q.WithOwnerTeam()
				}).
				All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].Edges.OwnerApplication.Edges.OwnerTeam.Name).To(Equal("team-alpha"))
		})

		It("should not leak team-beta's permission sets to a team-alpha-only viewer", func() {
			// team-beta viewer must not see team-alpha's permission set.
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{"team-beta"}})
			result, err := client.PermissionSet.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		// AgenticExposure/AgenticSubscription carry the same cross-tenant sensitivity
		// as their API/event counterparts, so verify the same leak-proof scoping here.
		It("should return only team-alpha's own agentic exposure, scoped correctly", func() {
			ctx := alphaCtx()
			result, err := client.AgenticExposure.Query().
				WithOwner(func(q *entgen.ApplicationQuery) {
					q.WithOwnerTeam()
				}).
				All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].Edges.Owner.Edges.OwnerTeam.Name).To(Equal("team-alpha"))
		})

		It("should not leak team-alpha's agentic exposure to a team-beta-only viewer", func() {
			// team-beta viewer must not see team-alpha's agentic exposure via a direct query,
			// even though team-beta legitimately subscribes to it (that access is only via
			// the reduced AgenticExposureInfo cross-tenant resolver, not a direct entity query).
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{"team-beta"}})
			result, err := client.AgenticExposure.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should not leak team-beta's agentic subscription to a team-alpha-only viewer", func() {
			// team-alpha viewer must not see team-beta's agentic subscription via a direct
			// query, even though team-alpha owns the target exposure (that access is only via
			// the reduced AgenticSubscriptionInfo cross-tenant resolver, not a direct entity query).
			ctx := alphaCtx()
			result, err := client.AgenticSubscription.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Context("when viewer belongs to both teams", func() {
		BeforeEach(func() { seed() })

		bothCtx := func() context.Context {
			return viewerCtx(&viewer.Viewer{Teams: []string{"team-alpha", "team-beta"}})
		}

		DescribeTable("should see all entities",
			func(queryAll func(context.Context) (int, error), expectedLen int) {
				count, err := queryAll(bothCtx())
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(expectedLen))
			},
			Entry("teams", func(ctx context.Context) (int, error) {
				r, e := client.Team.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("applications", func(ctx context.Context) (int, error) {
				r, e := client.Application.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("exposures", func(ctx context.Context) (int, error) {
				r, e := client.ApiExposure.Query().All(ctx)
				return len(r), e
			}, 2),
			Entry("subscriptions", func(ctx context.Context) (int, error) {
				r, e := client.ApiSubscription.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("event exposures", func(ctx context.Context) (int, error) {
				r, e := client.EventExposure.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("event subscriptions", func(ctx context.Context) (int, error) {
				r, e := client.EventSubscription.Query().All(ctx)
				return len(r), e
			}, 1),
			Entry("permission sets", func(ctx context.Context) (int, error) {
				r, e := client.PermissionSet.Query().All(ctx)
				return len(r), e
			}, 1),
		)
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

		It("should not filter mcp servers even for a different team", func() {
			// McpServerAlpha is owned by team-alpha, but a team-beta viewer should still see it
			// since McpServer is a public catalogue entity (mirrors Api/EventType).
			ctx := viewerCtx(&viewer.Viewer{Teams: []string{"team-beta"}})
			servers, err := client.McpServer.Query().All(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
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
