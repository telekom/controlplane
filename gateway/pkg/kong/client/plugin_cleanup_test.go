// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"errors"
	"net/http"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	v1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"
	mockclient "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CleanupPlugins", func() {
	const (
		routeScopeTags    = "env--test,route--r1"
		consumerScopeTags = "env--test,consumer--c1"
	)

	var (
		ctx      context.Context
		route    *v1.Route
		consumer *v1.Consumer
		api      *mockclient.MockKongAdminApi
		client   clientpkg.KongClient
		deleted  []string
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		route = &v1.Route{ObjectMeta: metav1.ObjectMeta{Name: "r1"}}
		consumer = &v1.Consumer{Spec: v1.ConsumerSpec{Name: "c1"}}
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
		deleted = nil
	})

	routeWide := func(id, name string) *pluginRecord {
		return &pluginRecord{
			id: id, name: name, route: routeID,
			tags: []string{"env--test", "plugin--" + name, "route--r1", "consumer--none"},
		}
	}
	combined := func(id string) *pluginRecord {
		return &pluginRecord{
			id: id, name: "rate-limiting-merged", route: routeID, consumer: consumerID,
			tags: []string{"env--test", "plugin--rate-limiting-merged", "route--r1", "consumer--c1"},
		}
	}
	consumerOnly := func(id string) *pluginRecord {
		return &pluginRecord{
			id: id, name: "ip-restriction", consumer: consumerID,
			tags: []string{"env--test", "plugin--ip-restriction", "consumer--c1"},
		}
	}

	Describe("route scope", func() {
		It("deletes obsolete route plugins in sorted order and keeps desired ones", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil,
				routeWide("b-stale", "acl").json(), routeWide("a-stale", "request-termination").json(), routeWide("kept", "jwt").json(),
			))
			expectDelete(api, "a-stale", http.StatusNoContent, &deleted)
			expectDelete(api, "b-stale", http.StatusNotFound, &deleted)

			Expect(client.CleanupPlugins(ctx, route, nil, []clientpkg.CustomPlugin{&testPlugin{id: "kept", name: "jwt"}})).To(Succeed())
			Expect(deleted).To(Equal([]string{"a-stale", "b-stale"}))
		})

		It("skips plugins without complete ownership proof", func() {
			bound := routeWide("service-bound", "acl")
			bound.service = serviceID
			legacy := &pluginRecord{id: "legacy", name: "acl", route: routeID, tags: []string{legacyTag, "route--r1", "env--test"}}
			foreignEnv := routeWide("foreign-env", "acl")
			foreignEnv.tags = []string{"env--prod", "plugin--acl", "route--r1", "consumer--none"}
			wrongName := routeWide("wrong-name", "acl")
			wrongName.tags = []string{"env--test", "plugin--jwt", "route--r1", "consumer--none"}
			otherRoute := routeWide("other-route", "acl")
			otherRoute.route = otherRouteID
			conflicting := routeWide("conflicting", "acl")
			conflicting.tags = append(conflicting.tags, "env--prod")
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil,
				bound.json(), legacy.json(), foreignEnv.json(), wrongName.json(), otherRoute.json(), conflicting.json(),
			))

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(Succeed())
		})

		It("deletes an obsolete combined plugin only with consumer proof", func() {
			mismatched := combined("mismatched")
			mismatched.tags = []string{"env--test", "plugin--rate-limiting-merged", "route--r1", "consumer--c2"}
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, combined("proven").json(), mismatched.json()))
			expectConsumerID(api, "c1", consumerID)
			expectConsumerID(api, "c2", "c2-id")
			expectDelete(api, "proven", http.StatusOK, &deleted)

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(Succeed())
			Expect(deleted).To(Equal([]string{"proven"}))
		})

		It("skips a combined plugin whose consumer Kong does not know", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, combined("unprovable").json()))
			expectConsumerStatus(api, "c1", http.StatusNotFound)

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(Succeed())
		})

		It("deletes nothing when a consumer lookup fails", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, routeWide("a-stale", "acl").json(), combined("b-combined").json()))
			expectConsumerStatus(api, "c1", http.StatusInternalServerError)

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(HaveOccurred())
			Expect(deleted).To(BeEmpty())
		})

		It("deletes nothing when a later page fails", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf("o1", routeWide("stale", "acl").json()))
			api.EXPECT().ListPluginWithResponse(mock.Anything, mock.Anything).Return(nil, errors.New("connection reset")).Once()

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(HaveOccurred())
			Expect(deleted).To(BeEmpty())
		})

		It("deletes nothing when a desired plugin has no ID", func() {
			Expect(client.CleanupPlugins(ctx, route, nil, []clientpkg.CustomPlugin{&testPlugin{name: "acl"}})).
				To(MatchError(ContainSubstring("has no id")))
		})

		It("rejects a malformed obsolete candidate", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, `{"id":"x","name":"acl","route":{}}`))

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(HaveOccurred())
		})

		It("rejects a record without ID", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, `{"name":"acl"}`))

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(MatchError(ContainSubstring("has no id")))
		})

		It("does not validate the associations of a desired plugin", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, `{"id":"kept","name":"acl","route":{}}`))

			Expect(client.CleanupPlugins(ctx, route, nil, []clientpkg.CustomPlugin{&testPlugin{id: "kept", name: "acl"}})).To(Succeed())
		})

		It("rejects duplicate candidate IDs", func() {
			record := routeWide("stale", "acl").json()
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, record, record))

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(MatchError(ContainSubstring("more than once")))
			Expect(deleted).To(BeEmpty())
		})

		It("reports a rejected delete", func() {
			expectRouteID(api)
			expectList(api, routeScopeTags, nil, pageOf(nil, routeWide("stale", "acl").json()))
			expectDelete(api, "stale", http.StatusInternalServerError, &deleted)

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(MatchError(ContainSubstring("failed to delete plugin")))
		})

		It("fails when the route does not exist", func() {
			api.EXPECT().GetRouteWithResponse(mock.Anything, "r1").Return(
				&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
			).Once()

			Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(HaveOccurred())
		})
	})

	Describe("consumer scope", func() {
		It("keeps route-owned combined plugins and deletes obsolete consumer plugins", func() {
			routeTagged := consumerOnly("route-tagged")
			routeTagged.tags = append(routeTagged.tags, "route--r1")
			expectConsumerID(api, "c1", consumerID)
			expectList(api, consumerScopeTags, nil, pageOf(nil,
				combined("combined").json(), routeTagged.json(), consumerOnly("stale").json(),
			))
			expectDelete(api, "stale", http.StatusNoContent, &deleted)

			Expect(client.CleanupPlugins(ctx, nil, consumer, nil)).To(Succeed())
			Expect(deleted).To(Equal([]string{"stale"}))
		})

		It("skips consumer plugins bound to another consumer", func() {
			other := consumerOnly("other")
			other.consumer = "other-consumer-id"
			expectConsumerID(api, "c1", consumerID)
			expectList(api, consumerScopeTags, nil, pageOf(nil, other.json()))

			Expect(client.CleanupPlugins(ctx, nil, consumer, nil)).To(Succeed())
		})
	})

	It("requires exactly one scope", func() {
		Expect(client.CleanupPlugins(ctx, nil, nil, nil)).To(MatchError(ContainSubstring("either route or consumer must be provided")))
		Expect(client.CleanupPlugins(ctx, route, consumer, nil)).To(MatchError(ContainSubstring("only one of route or consumer")))
	})
})
