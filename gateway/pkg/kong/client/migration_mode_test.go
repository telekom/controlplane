// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/changeset"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Migration mode", func() {
	var (
		ctx      context.Context
		kongFake *fakeKong
		client   clientpkg.KongClient
		route    *v1.Route
		upstream clientpkg.Upstream
		plan     *changeset.Plan
		planCtx  context.Context
	)

	kinds := func() []string {
		changes := plan.Changes()
		out := make([]string, 0, len(changes))
		for _, change := range changes {
			out = append(out, change.Kind+" "+change.Operation)
		}
		return out
	}

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "bond")
		kongFake = newFakeKong()
		guard, err := changeset.New(changeset.Options{Operator: "gateway", Mode: changeset.ModeOn, Kinds: clientpkg.EntityKinds}, changeset.Config{})
		Expect(err).NotTo(HaveOccurred())
		client = clientpkg.NewKongClient(kongFake.api(), clientpkg.WithMigrator(snapshotMigrator()), clientpkg.WithGate(guard))
		route = &v1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "eni-example-v1"},
			Spec:       v1.RouteSpec{Hostnames: []string{"stargate.example"}, Paths: []string{"/eni/echo/v3"}},
		}
		route.SetNamePrefix("bond")
		upstream = clientpkg.NewUpstreamOrDie("http://localhost:8080/proxy")
		plan = changeset.NewPlan()
		planCtx = changeset.WithPlan(ctx, plan)
	})

	Describe("with a legacy route", func() {
		BeforeEach(func() {
			snapshot := loadSnapshot("bond-api.yaml")
			kongFake.add("services", snapshot.Service)
			kongFake.add("routes", snapshot.Route)
		})

		It("plans the adoption of service and route without writing", func() {
			Expect(client.CreateOrReplaceRoute(planCtx, route, upstream)).To(Succeed())

			Expect(kongFake.writes()).To(BeEmpty())
			Expect(kinds()).To(Equal([]string{"service update", "route update"}))
			Expect(plan.Changes()[1].Diffs).To(ContainElement(HaveField("Path", "$.name")))
			Expect(route.GetRouteId()).To(Equal("legacy-route-id"))
		})

		It("writes the adoption while the plan is applied", func() {
			Expect(client.CreateOrReplaceRoute(changeset.WithApply(ctx), route, upstream)).To(Succeed())
			Expect(kongFake.get("routes", "legacy-route-id")["name"]).To(Equal("bond--eni-example-v1"))
		})

		It("plans the deletion without deleting", func() {
			Expect(client.DeleteRoute(planCtx, route)).To(Succeed())

			Expect(kongFake.writes()).To(BeEmpty())
			Expect(kinds()).To(Equal([]string{"route delete", "service delete"}))
		})

		It("blocks an unplanned write", func() {
			Expect(isBlocked(client.CreateOrReplaceRoute(ctx, route, upstream))).To(BeTrue())
			Expect(kongFake.writes()).To(BeEmpty())
		})
	})

	It("plans a new route with its plugins as one bundle", func() {
		Expect(client.CreateOrReplaceRoute(planCtx, route, upstream)).To(Succeed())
		Expect(changeset.IsPlanned(route.GetRouteId())).To(BeTrue())

		acl := plugin.AclPluginFromRoute(route)
		acl.Config.AddAllow("eni--team")
		Expect(client.CreateOrReplacePlugin(planCtx, acl)).Error().NotTo(HaveOccurred())

		Expect(kongFake.writes()).To(BeEmpty())
		Expect(kinds()).To(Equal([]string{"service create", "route create", "plugin create"}))
		Expect(plan.Changes()[1].Diffs[0].Desired).To(HaveKeyWithValue("service", map[string]any{"id": "planned--service--bond--eni-example-v1"}))
	})

	It("never records a plugin secret in clear", func() {
		Expect(client.CreateOrReplaceRoute(planCtx, route, upstream)).To(Succeed())

		jumperConfig := plugin.ToBase64OrDie(plugin.JumperConfig{OAuth: map[plugin.ConsumerId]plugin.OauthCredentials{
			"eni--team": {ClientSecret: "top-secret-client"},
		}})
		rtp := plugin.RequestTransformerPluginFromRoute(route)
		rtp.Config.Add.AddHeader(plugin.JumperConfigKey, jumperConfig)
		rtp.Config.Add.AddHeader(plugin.RoutingConfigKey, "visible")
		Expect(client.CreateOrReplacePlugin(planCtx, rtp)).Error().NotTo(HaveOccurred())

		rateLimit := plugin.RateLimitPluginFromRoute(route)
		rateLimit.Config.Password = "top-secret-redis"
		Expect(client.CreateOrReplacePlugin(planCtx, rateLimit)).Error().NotTo(HaveOccurred())

		recorded := render(plan.Status(plan.Hash(), changeset.ModeOn))
		Expect(recorded).NotTo(ContainSubstring("top-secret-redis"))
		Expect(recorded).NotTo(ContainSubstring(jumperConfig))
		Expect(recorded).To(ContainSubstring(plugin.JumperConfigKey + `:sha256:`))
		Expect(recorded).To(ContainSubstring("routing_config:visible"))
	})
})

// render returns the migration status as JSON, as it is stored on the object.
func render(status any) string {
	raw, err := json.Marshal(status)
	Expect(err).NotTo(HaveOccurred())
	return string(raw)
}
