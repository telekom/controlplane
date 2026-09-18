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
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Listener resolvers", func() {
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

	AfterEach(func() { client.Close() })

	It("resolves Listener fields and relationships", func() {
		ctx := testutil.AllowContext()
		resourceName, err := r.Listener().ResourceName(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(resourceName).To(Equal("api-alpha"))

		approved, err := r.Listener().Approved(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(approved).To(BeTrue())

		basePath, err := r.Listener().APIBasePath(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(basePath).To(Equal("/alpha"))
		Expect(s.ListenerReady.RequestFilter.Trigger).To(Equal(map[string]string{"method": "GET"}))
		requestFilter, err := r.Listener().RequestFilter(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(requestFilter.Payload).To(Equal([]string{"request.id"}))
		responseFilter, err := r.Listener().ResponseFilter(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(responseFilter.Payload).To(Equal([]string{"response.id"}))

		consumer, err := r.Listener().Consumer(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(consumer.Name).To(Equal("app-beta"))
		Expect(consumer.OwnerTeam.Name).To(Equal("team-beta"))

		application, err := r.Listener().Application(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(application).To(Equal(consumer))

		provider, err := r.Listener().Provider(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(provider.Name).To(Equal("app-alpha"))
		Expect(provider.OwnerTeam.Name).To(Equal("team-alpha"))

		subscription, err := r.Listener().Subscription(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(subscription.ID).To(Equal(s.Subscription.ID))
		exposure, err := r.Listener().Exposure(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(exposure.ID).To(Equal(s.ExposureAlpha.ID))
		providerApproval, err := r.Listener().ProviderApproval(ctx, s.ListenerReady)
		Expect(err).NotTo(HaveOccurred())
		Expect(providerApproval.ID).To(Equal(s.ListenerApproval.ID))

		pendingApproved, err := r.Listener().Approved(ctx, s.ListenerPending)
		Expect(err).NotTo(HaveOccurred())
		Expect(pendingApproved).To(BeFalse())
	})

	It("returns only ready Listeners from exposure and subscription fields", func() {
		ctx := testutil.AllowContext()
		exposureListeners, err := r.ApiExposure().Listeners(ctx, s.ExposureAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(exposureListeners).To(HaveLen(1))
		Expect(exposureListeners[0].ID).To(Equal(s.ListenerReady.ID))
		Expect(exposureListeners[0].ResourceName).To(Equal("api-alpha"))
		Expect(exposureListeners[0].Approved).To(BeTrue())

		subscriptionListeners, err := r.ApiSubscription().Listeners(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(subscriptionListeners).To(HaveLen(1))
		Expect(subscriptionListeners[0].ID).To(Equal(s.ListenerReady.ID))
	})

	It("includes non-ready Listeners on the consumer application", func() {
		connection, err := r.Application().Listeners(testutil.AllowContext(), s.AppBeta, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(connection.Edges).To(HaveLen(2))

		providerConnection, err := r.Application().Listeners(testutil.AllowContext(), s.AppAlpha, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(providerConnection.Edges).To(BeEmpty())
	})

	It("includes non-ready Listeners in top-level queries", func() {
		connection, err := r.Query().Listeners(testutil.AllowContext(), nil, nil, nil, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(connection.Edges).To(HaveLen(2))
	})

	It("maps filter values for GraphQL", func() {
		trigger, err := r.ListenerFilter().Trigger(context.Background(), s.ListenerReady.RequestFilter)
		Expect(err).NotTo(HaveOccurred())
		Expect(trigger).To(Equal(map[string]any{"method": "GET"}))

		payload, err := r.ListenerFilter().Payload(context.Background(), &model.ListenerFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(payload).NotTo(BeNil())
		Expect(payload).To(BeEmpty())
	})

	It("resolves Listener approval subscriptions", func() {
		ctx := testutil.AllowContext()
		subscription, err := r.Approval().Subscription(ctx, s.ListenerApproval)
		Expect(err).NotTo(HaveOccurred())
		Expect(subscription.(*model.ListenerInfo).ID).To(Equal(s.ListenerReady.ID))

		requestSubscription, err := r.ApprovalRequest().Subscription(ctx, s.ListenerApprovalRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(requestSubscription.(*model.ListenerInfo).ID).To(Equal(s.ListenerReady.ID))

		providerApproval, err := r.ApprovalRequest().Approval(ctx, s.ListenerApprovalRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(providerApproval.ID).To(Equal(s.ListenerApproval.ID))
	})

	It("preserves API and event approval subscription behavior", func() {
		ctx := testutil.AllowContext()
		apiTarget, err := r.Approval().Subscription(ctx, s.Approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(apiTarget.(*model.ApiSubscriptionInfo).ID).To(Equal(s.Subscription.ID))
		apiRequestTarget, err := r.ApprovalRequest().Subscription(ctx, s.ApprovalRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(apiRequestTarget.(*model.ApiSubscriptionInfo).ID).To(Equal(s.Subscription.ID))

		eventApproval := client.Approval.Create().
			SetNamespace("prod").SetName("event-approval").SetAction("ALLOW").
			SetRequester(model.RequesterInfo{TeamName: "team-beta"}).
			SetDecider(model.DeciderInfo{TeamName: "team-alpha"}).
			SetDeciderTeamName("team-alpha").SetEventSubscription(s.EventSubscription).SaveX(ctx)
		eventTarget, err := r.Approval().Subscription(ctx, eventApproval)
		Expect(err).NotTo(HaveOccurred())
		Expect(eventTarget.(*model.EventSubscriptionInfo).ID).To(Equal(s.EventSubscription.ID))
	})

	It("deletes Listener-targeted approval records with the Listener", func() {
		ctx := testutil.AllowContext()
		client.Listener.DeleteOne(s.ListenerReady).ExecX(ctx)
		_, err := client.Approval.Get(ctx, s.ListenerApproval.ID)
		Expect(ent.IsNotFound(err)).To(BeTrue())
		_, err = client.ApprovalRequest.Get(ctx, s.ListenerApprovalRequest.ID)
		Expect(ent.IsNotFound(err)).To(BeTrue())
	})
})
