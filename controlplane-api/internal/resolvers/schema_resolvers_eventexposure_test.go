// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/eventexposure"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	gqlmodel "github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventExposure resolver", func() {
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

	It("should persist and retrieve event scopes with all fields", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.EventExposure.Create().
			SetNamespace("default").
			SetEventType("order.example").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{
				Strategy:     "FOUR_EYES",
				TrustedTeams: []string{"team-beta", "team-gamma"},
			}).
			SetEventScopes([]model.EventScope{
				{
					Name: "scope1",
					Trigger: model.EventTrigger{
						ResponseFilter: &model.ResponseFilter{
							Paths: []string{"$.data.id", "$.data.name"},
							Mode:  "Include",
						},
						SelectionFilter: &model.SelectionFilter{
							Attributes: map[string]string{"source": "order-service"},
							Expression: `{"op":"eq","path":"$.type","value":"order.created"}`,
						},
					},
				},
				{
					Name: "scope2",
					Trigger: model.EventTrigger{
						ResponseFilter: &model.ResponseFilter{
							Paths: []string{"$.data.status"},
							Mode:  "Exclude",
						},
					},
				},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Query back and assert all nested fields are persisted correctly.
		exp, err := client.EventExposure.Query().
			Where(eventexposure.EventTypeEQ("order.example")).
			Only(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(exp.EventType).To(Equal("order.example"))
		Expect(exp.ApprovalConfig.Strategy).To(Equal("FOUR_EYES"))
		Expect(exp.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-beta", "team-gamma"}))

		// Scopes assertions.
		Expect(exp.EventScopes).To(HaveLen(2))

		// Scope 1: full trigger with both filters.
		scope1 := exp.EventScopes[0]
		Expect(scope1.Name).To(Equal("scope1"))
		Expect(scope1.Trigger.ResponseFilter).NotTo(BeNil())
		Expect(scope1.Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.id", "$.data.name"}))
		Expect(scope1.Trigger.ResponseFilter.Mode).To(Equal("Include"))
		Expect(scope1.Trigger.SelectionFilter).NotTo(BeNil())
		Expect(scope1.Trigger.SelectionFilter.Attributes).To(HaveKeyWithValue("source", "order-service"))
		Expect(scope1.Trigger.SelectionFilter.Expression).To(Equal(`{"op":"eq","path":"$.type","value":"order.created"}`))

		// Scope 2: trigger with only response filter, no selection filter.
		scope2 := exp.EventScopes[1]
		Expect(scope2.Name).To(Equal("scope2"))
		Expect(scope2.Trigger.ResponseFilter).NotTo(BeNil())
		Expect(scope2.Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.status"}))
		Expect(scope2.Trigger.ResponseFilter.Mode).To(Equal("Exclude"))
		Expect(scope2.Trigger.SelectionFilter).To(BeNil())
	})

	Describe("ResponseFilter.Mode resolver", func() {
		It("should convert Include string to ResponseFilterMode", func() {
			mode, err := r.ResponseFilter().Mode(context.Background(), &model.ResponseFilter{Mode: "INCLUDE"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mode).NotTo(BeNil())
			Expect(*mode).To(Equal(gqlmodel.ResponseFilterModeInclude))
		})

		It("should convert Exclude string to ResponseFilterMode", func() {
			mode, err := r.ResponseFilter().Mode(context.Background(), &model.ResponseFilter{Mode: "EXCLUDE"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mode).NotTo(BeNil())
			Expect(*mode).To(Equal(gqlmodel.ResponseFilterModeExclude))
		})

		It("should return nil for empty mode", func() {
			mode, err := r.ResponseFilter().Mode(context.Background(), &model.ResponseFilter{Mode: ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(mode).To(BeNil())
		})
	})

	Describe("SelectionFilter.Attributes resolver", func() {
		It("should convert map[string]string to map[string]any", func() {
			attrs, err := r.SelectionFilter().Attributes(context.Background(), &model.SelectionFilter{
				Attributes: map[string]string{"source": "order-service", "type": "order.created"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(attrs).To(HaveLen(2))
			Expect(attrs["source"]).To(Equal("order-service"))
			Expect(attrs["type"]).To(Equal("order.created"))
		})

		It("should return nil for nil attributes", func() {
			attrs, err := r.SelectionFilter().Attributes(context.Background(), &model.SelectionFilter{
				Attributes: nil,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(attrs).To(BeNil())
		})
	})
})
