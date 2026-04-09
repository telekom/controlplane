// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

var _ = Describe("Subscriptions resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, nil)
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return ApiSubscriptionInfo for an exposure's subscriptions", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		subs, err := r.ApiExposure().Subscriptions(ctx, s.ExposureAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(subs).To(HaveLen(1))
		Expect(subs[0].BasePath).To(Equal("/alpha"))
		Expect(subs[0].OwnerApplicationName).To(Equal("app-beta"))
		Expect(subs[0].OwnerTeam).NotTo(BeNil())
		Expect(subs[0].OwnerTeam.Name).To(Equal("team-beta"))
		Expect(subs[0].OwnerTeam.GroupName).To(Equal("group-b"))
	})

	It("should return empty list when no subscriptions exist", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-beta"}})
		subs, err := r.ApiExposure().Subscriptions(ctx, s.ExposureBeta)
		Expect(err).NotTo(HaveOccurred())
		Expect(subs).To(BeEmpty())
	})
})

var _ = Describe("Target resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, nil)
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return ApiExposureInfo for a subscription's target", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-beta"}})
		info, err := r.ApiSubscription().Target(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		Expect(info.BasePath).To(Equal("/alpha"))
		Expect(info.OwnerApplicationName).To(Equal("app-alpha"))
		Expect(info.OwnerTeam).NotTo(BeNil())
		Expect(info.OwnerTeam.Name).To(Equal("team-alpha"))
		Expect(info.OwnerTeam.GroupName).To(Equal("group-a"))
	})
})

var _ = Describe("Approval.APISubscription resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, nil)
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return ApiSubscriptionInfo from an approval", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := r.Approval().APISubscription(ctx, s.Approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		Expect(info.BasePath).To(Equal("/alpha"))
		Expect(info.OwnerApplicationName).To(Equal("app-beta"))
		Expect(info.OwnerTeam.Name).To(Equal("team-beta"))
	})
})

var _ = Describe("ApprovalRequest.APISubscription resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, nil)
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return ApiSubscriptionInfo from an approval request", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := r.ApprovalRequest().APISubscription(ctx, s.ApprovalRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		Expect(info.BasePath).To(Equal("/alpha"))
		Expect(info.OwnerApplicationName).To(Equal("app-beta"))
		Expect(info.OwnerTeam.Name).To(Equal("team-beta"))
	})
})

var _ = Describe("ApiExposureInfo resolvers", func() {
	r := resolvers.NewResolver(nil, nil)

	It("should convert visibility string to enum", func() {
		v, err := r.ApiExposureInfo().Visibility(context.TODO(), &model.ApiExposureInfo{Visibility: "WORLD"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(v)).To(Equal("WORLD"))
	})

	It("should convert feature strings to enum array", func() {
		features, err := r.ApiExposureInfo().Features(context.TODO(), &model.ApiExposureInfo{
			Features: []string{"BASIC_AUTH", "CIRCUIT_BREAKER"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(features).To(HaveLen(2))
		Expect(string(features[0])).To(Equal("BASIC_AUTH"))
		Expect(string(features[1])).To(Equal("CIRCUIT_BREAKER"))
	})

	It("should return empty features for nil list", func() {
		features, err := r.ApiExposureInfo().Features(context.TODO(), &model.ApiExposureInfo{})
		Expect(err).NotTo(HaveOccurred())
		Expect(features).To(BeEmpty())
	})
})

var _ = Describe("ApiSubscriptionInfo.StatusPhase resolver", func() {
	r := resolvers.NewResolver(nil, nil)

	It("should convert status phase string to enum", func() {
		sp := "SUBSCRIBED"
		phase, err := r.ApiSubscriptionInfo().StatusPhase(context.TODO(), &model.ApiSubscriptionInfo{StatusPhase: &sp})
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).NotTo(BeNil())
		Expect(string(*phase)).To(Equal("SUBSCRIBED"))
	})

	It("should return nil for nil status phase", func() {
		phase, err := r.ApiSubscriptionInfo().StatusPhase(context.TODO(), &model.ApiSubscriptionInfo{})
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).To(BeNil())
	})
})
