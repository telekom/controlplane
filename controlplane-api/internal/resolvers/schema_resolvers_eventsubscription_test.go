// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/ent"
	enteventsubscription "github.com/telekom/controlplane/controlplane-api/ent/eventsubscription"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	gqlmodel "github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventSubscription resolver", func() {
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

	It("should persist and retrieve event subscription with trigger and delivery", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-beta").SetEmail("b@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-beta").SetClientID("cid-beta").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		callbackURL := "https://consumer.example.com/events"
		sub, err := client.EventSubscription.Create().
			SetNamespace("default").
			SetName("my-subscription").
			SetEventType("de.telekom.order.v1").
			SetDeliveryType(enteventsubscription.DeliveryTypeCallback).
			SetCallbackURL(callbackURL).
			SetOwner(app).
			SetScopes([]string{"scope-a", "scope-b"}).
			SetTrigger(&model.EventTrigger{
				ResponseFilter: &model.ResponseFilter{
					Paths: []string{"$.data.id", "$.data.status"},
					Mode:  "Include",
				},
				SelectionFilter: &model.SelectionFilter{
					Attributes: map[string]string{"source": "order-service", "type": "order.created"},
					Expression: `{"op":"eq","path":"$.type","value":"order.created"}`,
				},
			}).
			SetDelivery(model.EventDelivery{
				Payload:               "Data",
				EventRetentionTime:    "7d",
				CircuitBreakerOptOut:  true,
				RetryableStatusCodes:  []int{502, 503},
				RedeliveriesPerSecond: intPtr(10),
				EnforceGetHttpRequestMethodForHealthCheck: true,
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.EventType).To(Equal("de.telekom.order.v1"))
		Expect(sub.DeliveryType.String()).To(Equal("CALLBACK"))
		Expect(sub.CallbackURL).ToNot(BeNil())
		Expect(*sub.CallbackURL).To(Equal(callbackURL))
		Expect(sub.Scopes).To(Equal([]string{"scope-a", "scope-b"}))
		Expect(sub.Trigger).ToNot(BeNil())
		Expect(sub.Trigger.ResponseFilter).ToNot(BeNil())
		Expect(sub.Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.id", "$.data.status"}))
		Expect(sub.Trigger.ResponseFilter.Mode).To(Equal("Include"))
		Expect(sub.Trigger.SelectionFilter).ToNot(BeNil())
		Expect(sub.Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"source": "order-service", "type": "order.created"}))
		Expect(sub.Trigger.SelectionFilter.Expression).To(Equal(`{"op":"eq","path":"$.type","value":"order.created"}`))
		Expect(sub.Delivery.Payload).To(Equal("Data"))
		Expect(sub.Delivery.EventRetentionTime).To(Equal("7d"))
		Expect(sub.Delivery.CircuitBreakerOptOut).To(BeTrue())
		Expect(sub.Delivery.RetryableStatusCodes).To(Equal([]int{502, 503}))
		Expect(sub.Delivery.RedeliveriesPerSecond).ToNot(BeNil())
		Expect(*sub.Delivery.RedeliveriesPerSecond).To(Equal(10))
		Expect(sub.Delivery.EnforceGetHttpRequestMethodForHealthCheck).To(BeTrue())
	})

	It("should persist subscription with ServerSentEvent delivery and nil trigger fields", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-gamma").SetEmail("g@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-gamma").SetClientID("cid-gamma").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.EventSubscription.Create().
			SetNamespace("default").
			SetName("sse-subscription").
			SetEventType("de.telekom.sse.v1").
			SetDeliveryType(enteventsubscription.DeliveryTypeServerSentEvent).
			SetOwner(app).
			SetTrigger(&model.EventTrigger{}).
			SetDelivery(model.EventDelivery{
				Payload: "Data",
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.DeliveryType.String()).To(Equal("SERVER_SENT_EVENT"))
		Expect(sub.CallbackURL).To(BeNil())
		Expect(sub.Trigger.ResponseFilter).To(BeNil())
		Expect(sub.Trigger.SelectionFilter).To(BeNil())
	})

	Describe("EventSubscriptionInfo.DeliveryType resolver", func() {
		It("should convert CALLBACK string to DeliveryType enum", func() {
			dt, err := r.EventSubscriptionInfo().DeliveryType(context.Background(), &model.EventSubscriptionInfo{
				DeliveryType: "CALLBACK",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(dt).To(Equal(enteventsubscription.DeliveryTypeCallback))
		})

		It("should convert SERVER_SENT_EVENT string to DeliveryType enum", func() {
			dt, err := r.EventSubscriptionInfo().DeliveryType(context.Background(), &model.EventSubscriptionInfo{
				DeliveryType: "SERVER_SENT_EVENT",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(dt).To(Equal(enteventsubscription.DeliveryTypeServerSentEvent))
		})
	})

	Describe("EventSubscriptionInfo.StatusPhase resolver", func() {
		It("should convert READY string to StatusPhase enum", func() {
			phase := "READY"
			sp, err := r.EventSubscriptionInfo().StatusPhase(context.Background(), &model.EventSubscriptionInfo{
				StatusPhase: &phase,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(sp).ToNot(BeNil())
			Expect(sp.String()).To(Equal("READY"))
		})

		It("should return nil for nil StatusPhase", func() {
			sp, err := r.EventSubscriptionInfo().StatusPhase(context.Background(), &model.EventSubscriptionInfo{
				StatusPhase: nil,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(sp).To(BeNil())
		})
	})

	Describe("EventDelivery.Payload resolver", func() {
		It("should convert DATA string to PayloadType enum", func() {
			payload, err := r.EventDelivery().Payload(context.Background(), &model.EventDelivery{
				Payload: "DATA",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(payload).To(Equal(gqlmodel.PayloadTypeData))
		})

		It("should convert DATA_REF string to PayloadType enum", func() {
			payload, err := r.EventDelivery().Payload(context.Background(), &model.EventDelivery{
				Payload: "DATA_REF",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(payload).To(Equal(gqlmodel.PayloadTypeDataRef))
		})
	})
})

func intPtr(i int) *int {
	return &i
}
