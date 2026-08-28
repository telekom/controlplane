// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Root query resolvers for agentic entities", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should paginate mcpServers without panicking", func() {
		ctx := testutil.AllowContext()
		conn, err := r.Query().McpServers(ctx, nil, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(conn).NotTo(BeNil())
		Expect(conn.Edges).To(HaveLen(1))
	})

	It("should paginate agentCards without panicking", func() {
		ctx := testutil.AllowContext()
		conn, err := r.Query().AgentCards(ctx, nil, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(conn).NotTo(BeNil())
		Expect(conn.Edges).To(BeEmpty())
	})

	It("should paginate agenticExposures without panicking", func() {
		ctx := testutil.AllowContext()
		conn, err := r.Query().AgenticExposures(ctx, nil, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(conn).NotTo(BeNil())
		Expect(conn.Edges).To(HaveLen(1))
	})

	It("should paginate agenticSubscriptions without panicking", func() {
		ctx := testutil.AllowContext()
		conn, err := r.Query().AgenticSubscriptions(ctx, nil, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(conn).NotTo(BeNil())
		Expect(conn.Edges).To(HaveLen(1))
	})

	It("should respect the first/after pagination window and report hasNextPage correctly", func() {
		ctx := testutil.AllowContext()

		// SeedStandard already created one McpServer; add two more so there are
		// three total, enough to exercise a real pagination window.
		_, err := client.McpServer.Create().
			SetNamespace("default").SetBasePath("/mcp-b").SetVersion("1.0.0").SetName("mcp-b").
			SetOwner(s.TeamAlpha).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.McpServer.Create().
			SetNamespace("default").SetBasePath("/mcp-c").SetVersion("1.0.0").SetName("mcp-c").
			SetOwner(s.TeamAlpha).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		first := 2
		page1, err := r.Query().McpServers(ctx, nil, &first, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(page1.Edges).To(HaveLen(2))
		Expect(page1.TotalCount).To(Equal(3))
		Expect(page1.PageInfo.HasNextPage).To(BeTrue())

		after := page1.PageInfo.EndCursor
		page2, err := r.Query().McpServers(ctx, after, &first, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(page2.Edges).To(HaveLen(1))
		Expect(page2.PageInfo.HasNextPage).To(BeFalse())
	})
})

var _ = Describe("McpServer resolvers", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "https://files.example.com")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return the active exposure for a mcp server", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		info, err := r.McpServer().ActiveExposure(ctx, s.McpServerAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		Expect(info.BasePath).To(Equal("/mcp-alpha"))
		Expect(info.OwnerApplicationName).To(Equal("app-alpha"))
		Expect(info.OwnerTeam).NotTo(BeNil())
		Expect(info.OwnerTeam.Name).To(Equal("team-alpha"))
	})

	It("should return nil when no active exposure exists", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		other, err := client.McpServer.Create().
			SetNamespace("default").
			SetBasePath("/mcp-noexp").
			SetVersion("1.0.0").
			SetName("mcp-noexp").
			SetOwner(s.TeamAlpha).
			Save(testutil.AllowContext())
		Expect(err).NotTo(HaveOccurred())

		info, err := r.McpServer().ActiveExposure(ctx, other)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).To(BeNil())
	})

	It("should resolve the owner team", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		team, err := r.McpServer().Owner(ctx, s.McpServerAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(team).NotTo(BeNil())
		Expect(team.Name).To(Equal("team-alpha"))
		Expect(team.GroupName).To(Equal("group-a"))
	})

	It("should build the specification url", func() {
		ctx := testutil.AllowContext()
		withSpec, err := client.McpServer.Create().
			SetNamespace("default").
			SetBasePath("/mcp-spec").
			SetVersion("1.0.0").
			SetName("mcp-spec").
			SetSpecification("file-id-123").
			SetOwner(s.TeamAlpha).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		url, err := r.McpServer().SpecificationURL(ctx, withSpec)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).NotTo(BeNil())
		Expect(*url).To(Equal("https://files.example.com/files/file-id-123"))
	})

	It("should return nil specification url when specification is empty", func() {
		ctx := testutil.AllowContext()
		url, err := r.McpServer().SpecificationURL(ctx, s.McpServerAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())
	})
})

var _ = Describe("AgenticExposure.Subscriptions resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return AgenticSubscriptionInfo for an exposure's subscriptions", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		subs, err := r.AgenticExposure().Subscriptions(ctx, s.AgenticExposureAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(subs).To(HaveLen(1))
		Expect(subs[0].BasePath).To(Equal("/mcp-alpha"))
		Expect(subs[0].OwnerApplicationName).To(Equal("app-beta"))
		Expect(subs[0].OwnerTeam).NotTo(BeNil())
		Expect(subs[0].OwnerTeam.Name).To(Equal("team-beta"))
	})
})

var _ = Describe("AgenticSubscription.Target resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return AgenticExposureInfo for a subscription's target", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-beta"}})
		info, err := r.AgenticSubscription().Target(ctx, s.AgenticSubscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		Expect(info.BasePath).To(Equal("/mcp-alpha"))
		Expect(info.OwnerApplicationName).To(Equal("app-alpha"))
		Expect(info.OwnerTeam).NotTo(BeNil())
		Expect(info.OwnerTeam.Name).To(Equal("team-alpha"))
	})
})

var _ = Describe("Approval.AgenticSubscription resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return AgenticSubscriptionInfo from an approval", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := r.Approval().Subscription(ctx, s.AgenticApproval)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		agenticInfo, ok := info.(*model.AgenticSubscriptionInfo)
		Expect(ok).To(BeTrue(), "expected AgenticSubscriptionInfo union member")
		Expect(agenticInfo.BasePath).To(Equal("/mcp-alpha"))
		Expect(agenticInfo.OwnerApplicationName).To(Equal("app-beta"))
		Expect(agenticInfo.OwnerTeam.Name).To(Equal("team-beta"))
	})
})

var _ = Describe("ApprovalRequest.AgenticSubscription resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return AgenticSubscriptionInfo from an approval request", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := r.ApprovalRequest().Subscription(ctx, s.AgenticApprovalRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		agenticInfo, ok := info.(*model.AgenticSubscriptionInfo)
		Expect(ok).To(BeTrue(), "expected AgenticSubscriptionInfo union member")
		Expect(agenticInfo.BasePath).To(Equal("/mcp-alpha"))
		Expect(agenticInfo.OwnerApplicationName).To(Equal("app-beta"))
		Expect(agenticInfo.OwnerTeam.Name).To(Equal("team-beta"))
	})

	It("should resolve the approval from an approval request", func() {
		ctx := testutil.AllowContext()
		appr, err := r.ApprovalRequest().Approval(ctx, s.AgenticApprovalRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(appr).NotTo(BeNil())
		Expect(appr.ID).To(Equal(s.AgenticApproval.ID))
	})
})

var _ = Describe("AgenticExposureInfo resolvers", func() {
	r := resolvers.NewResolver(nil, service.Services{}, nil, "")

	It("should convert visibility string to enum", func() {
		v, err := r.AgenticExposureInfo().Visibility(context.TODO(), &model.AgenticExposureInfo{Visibility: "WORLD"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(v)).To(Equal("WORLD"))
	})

	It("should convert variant string to enum", func() {
		v, err := r.AgenticExposureInfo().Variant(context.TODO(), &model.AgenticExposureInfo{Variant: "AGENT"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(v)).To(Equal("AGENT"))
	})
})

var _ = Describe("AgenticSubscriptionInfo.StatusPhase resolver", func() {
	r := resolvers.NewResolver(nil, service.Services{}, nil, "")

	It("should convert status phase string to enum", func() {
		sp := "READY"
		phase, err := r.AgenticSubscriptionInfo().StatusPhase(context.TODO(), &model.AgenticSubscriptionInfo{StatusPhase: &sp})
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).NotTo(BeNil())
		Expect(string(*phase)).To(Equal("READY"))
	})

	It("should return nil for nil status phase", func() {
		phase, err := r.AgenticSubscriptionInfo().StatusPhase(context.TODO(), &model.AgenticSubscriptionInfo{})
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).To(BeNil())
	})
})
