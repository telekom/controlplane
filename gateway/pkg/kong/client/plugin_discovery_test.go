// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"errors"
	"net/http"

	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"
	mockclient "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	legacyTag     = "ei__telekom__de--realm---default"
	routeTags     = "env--test,plugin--acl,route--r1,consumer--none"
	combinedTags  = "env--test,plugin--acl,route--r1,consumer--c1"
	consumerTags  = "env--test,plugin--acl,consumer--c1"
	emptyPage     = `{"data":[]}`
	routeID       = "route-id"
	consumerID    = "consumer-id"
	legacyID      = "legacy-id"
	storedID      = "stored-id"
	otherRouteID  = "other-route-id"
	serviceID     = "service-id"
	unrelatedName = "jwt"
)

var routeGoTags = []string{"env--test", "plugin--acl", "route--r1", "consumer--none"}

// Legacy plugins carry the realm tag, the plugin name and their bindings.
var (
	legacyRouteTags    = []string{legacyTag, "acl", "r1"}
	legacyCombinedTags = []string{legacyTag, "acl", "r1", "c1"}
)

var _ = Describe("Bound plugin discovery", func() {
	var (
		ctx     context.Context
		api     *mockclient.MockKongAdminApi
		client  clientpkg.KongClient
		plugin  *testPlugin
		written writtenBody
	)

	allowConfig := map[string]any{"allow": []any{"group-a"}}

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
		plugin = &testPlugin{name: "acl", route: ptr("r1"), config: map[string]any{"allow": []any{"group-a"}}}
		written = writtenBody{}
	})

	legacyRoutePlugin := func() *pluginRecord {
		return &pluginRecord{
			id: legacyID, name: "acl", route: routeID, tags: legacyRouteTags,
			protocols: []string{"grpc", "grpcs", "http", "https"}, instanceName: "legacy-instance", config: allowConfig,
		}
	}

	Describe("lookup order", func() {
		It("falls back from a missing stored ID to the Go tags and then to the slot on the route", func() {
			plugin.id = storedID
			expectRouteID(api)
			expectPluginStatus(api, storedID, http.StatusNotFound)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil, legacyRoutePlugin().json()))
			expectRouteWrite(api, &written)

			result, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(*result.Id).To(Equal(legacyID))
			Expect(plugin.GetId()).To(Equal(legacyID))

			By("updating the same ID with additive tags although the config is equal")
			Expect(written.id).To(Equal(legacyID))
			Expect(*written.body.Tags).To(ConsistOf(append(routeGoTags, legacyRouteTags...)))
			Expect(*written.body.Protocols).To(Equal([]kong.CreatePluginForConsumerRequestProtocols{"grpc", "grpcs", "http", "https"}))
			Expect(*written.body.InstanceName).To(Equal("legacy-instance"))
			Expect(written.body.Service).To(BeNil())
		})

		It("does not query the slot when the Go tags find the plugin", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, pageOf(nil, (&pluginRecord{
				id: storedID, name: "acl", route: routeID, tags: routeGoTags, protocols: []string{"http"}, config: allowConfig,
			}).json()))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(plugin.GetId()).To(Equal(storedID))
		})

		It("creates a new plugin without the legacy tag when the slot is free", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, emptyPage)
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(*written.body.Tags).NotTo(ContainElement(legacyTag))
			Expect(*written.body.Tags).To(ConsistOf(routeGoTags))
		})

		It("stops on a stored ID read failure other than 404", func() {
			plugin.id = storedID
			expectRouteID(api)
			expectPluginStatus(api, storedID, http.StatusInternalServerError)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(HaveOccurred())
			Expect(plugin.GetId()).To(Equal(storedID))
		})

		It("stops on a stored ID transport failure", func() {
			plugin.id = storedID
			expectRouteID(api)
			api.EXPECT().GetPluginWithResponse(mock.Anything, storedID).Return(nil, errors.New("connection reset")).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(HaveOccurred())
		})

		It("stops on a Go tag listing failure without querying the slot", func() {
			expectRouteID(api)
			api.EXPECT().ListPluginWithResponse(mock.Anything, mock.Anything).Return(nil, errors.New("list failed")).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("parent resolution", func() {
		It("fails when the route does not exist", func() {
			api.EXPECT().GetRouteWithResponse(mock.Anything, "r1").Return(
				&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
			).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("does not exist")))
		})

		It("fails when the consumer has no ID", func() {
			plugin.route, plugin.consumer = nil, ptr("c1")
			api.EXPECT().GetConsumerWithResponse(mock.Anything, "c1").Return(
				&kong.GetConsumerResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Consumer{}}, nil,
			).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("response ID is missing")))
		})

		It("fails when the route lookup fails", func() {
			api.EXPECT().GetRouteWithResponse(mock.Anything, "r1").Return(nil, errors.New("timeout")).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("legacy identity", func() {
		It("has no legacy fallback for a consumer-only plugin", func() {
			plugin.route, plugin.consumer = nil, ptr("c1")
			expectConsumerID(api, "c1", consumerID)
			expectList(api, consumerTags, nil, emptyPage)
			expectConsumerWrite(api, "c1", &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).NotTo(Equal(legacyID))
			Expect(*written.body.Tags).To(ConsistOf("env--test", "plugin--acl", "consumer--c1"))
		})

		It("adopts a combined legacy plugin bound to exactly the route and consumer", func() {
			plugin.consumer = ptr("c1")
			expectRouteID(api)
			expectConsumerID(api, "c1", consumerID)
			expectList(api, combinedTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil,
				(&pluginRecord{id: "route-wide", name: "acl", route: routeID, tags: legacyCombinedTags}).json(),
				(&pluginRecord{id: "consumer-wide", name: "acl", consumer: consumerID, tags: legacyCombinedTags}).json(),
				(&pluginRecord{id: legacyID, name: "acl", route: routeID, consumer: consumerID, tags: legacyCombinedTags}).json(),
			))
			expectConsumerWrite(api, "c1", &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).To(Equal(legacyID))
			Expect(*written.body.Consumer).To(Equal("c1"))
			Expect(*written.body.Route).To(Equal(map[string]any{"name": ptr("r1")}))
			Expect(*written.body.Tags).To(ConsistOf(legacyTag, "acl", "r1", "c1", "env--test", "plugin--acl", "route--r1", "consumer--c1"))
		})

		It("ignores legacy plugins of another slot, another name or bound to a service", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil,
				(&pluginRecord{id: "a", name: "acl", route: otherRouteID, tags: legacyRouteTags}).json(),
				(&pluginRecord{id: "b", name: "acl", route: routeID, consumer: consumerID, tags: append(legacyRouteTags, "c1")}).json(),
				(&pluginRecord{id: "c", name: "acl", route: routeID, service: serviceID, tags: legacyRouteTags}).json(),
				`{"name":"`+unrelatedName+`"}`,
			))
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).NotTo(BeElementOf("a", "b", "c"))
		})

		It("blocks on an in-slot legacy plugin with conflicting ownership tags", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil, (&pluginRecord{
				id: legacyID, name: "acl", route: routeID, tags: append(legacyRouteTags, "route--other"),
			}).json()))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("is not owned")))
			Expect(isBlocked(err)).To(BeTrue())
			Expect(plugin.GetId()).To(BeEmpty())
		})

		It("adopts an in-slot plugin of any legacy realm", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil, (&pluginRecord{
				id: legacyID, name: "acl", route: routeID, tags: []string{"ei__telekom__de--realm---bond", "acl", "bond.r1"}, config: allowConfig,
			}).json()))
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).To(Equal(legacyID))
			Expect(*written.body.Tags).To(ContainElements("ei__telekom__de--realm---bond", "acl", "bond.r1"))
		})

		It("blocks on an in-slot plugin without legacy realm tag or Go tags", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil, (&pluginRecord{
				id: "foreign", name: "acl", route: routeID, tags: []string{"acl", "r1"},
			}).json()))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("occupies the slot")))
			Expect(isBlocked(err)).To(BeTrue())
			Expect(plugin.GetId()).To(BeEmpty())
		})

		It("blocks on an untagged in-slot plugin", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil, (&pluginRecord{id: "foreign", name: "acl", route: routeID}).json()))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(isBlocked(err)).To(BeTrue())
		})

		It("stops on a slot listing failure", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			api.EXPECT().ListPluginsForRouteWithResponse(mock.Anything, routeID, mock.Anything).Return(nil, errors.New("list failed")).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("list failed")))
		})

		It("rejects ambiguous legacy plugins in the same slot", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf(nil,
				(&pluginRecord{id: "a", name: "acl", route: routeID, tags: legacyRouteTags}).json(),
				(&pluginRecord{id: "b", name: "acl", route: routeID, tags: legacyRouteTags}).json(),
			))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("found multiple")))
		})

		It("accepts a stored legacy ID with partial Go tags", func() {
			plugin.id = legacyID
			expectRouteID(api)
			expectPluginRecord(api, legacyID, &pluginRecord{
				id: legacyID, name: "acl", route: routeID, tags: []string{legacyTag, "env--test", "plugin--acl"}, config: allowConfig,
			})
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).To(Equal(legacyID))
		})

		It("rejects a stored legacy ID bound to another route", func() {
			plugin.id = legacyID
			expectRouteID(api)
			expectPluginRecord(api, legacyID, &pluginRecord{id: legacyID, name: "acl", route: otherRouteID, tags: []string{legacyTag}})

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("is not owned")))
			Expect(plugin.GetId()).To(Equal(legacyID))
		})
	})

	Describe("stored ID and Go tag ownership", func() {
		It("rejects a stored ID that belongs to another plugin", func() {
			plugin.id = storedID
			expectRouteID(api)
			expectPluginRecord(api, storedID, &pluginRecord{id: storedID, name: unrelatedName, route: routeID, tags: routeGoTags})

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("is not owned")))
		})

		It("rejects a stored ID whose record reports a different ID", func() {
			plugin.id = storedID
			expectRouteID(api)
			expectPluginRecord(api, storedID, &pluginRecord{id: "other", name: "acl", route: routeID, tags: routeGoTags})

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("unrelated id")))
		})

		It("rejects a stored ID bound to a service", func() {
			plugin.id = storedID
			expectRouteID(api)
			expectPluginRecord(api, storedID, &pluginRecord{id: storedID, name: "acl", route: routeID, service: serviceID, tags: routeGoTags})

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("is not owned")))
		})

		It("repairs a drifted route association of a Go-owned plugin under the same ID", func() {
			plugin.id = storedID
			expectRouteID(api)
			expectPluginRecord(api, storedID, &pluginRecord{
				id: storedID, name: "acl", route: otherRouteID, tags: routeGoTags, protocols: []string{"http"}, config: allowConfig,
			})
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).To(Equal(storedID))
		})

		It("repairs a Go-owned plugin found by tags without a route association", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, pageOf(nil, (&pluginRecord{
				id: storedID, name: "acl", tags: routeGoTags, protocols: []string{"http"}, config: allowConfig,
			}).json()))
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).To(Equal(storedID))
		})

		It("rejects a Go tag hit with a conflicting ownership tag", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, pageOf(nil, (&pluginRecord{
				id: storedID, name: "acl", route: routeID, tags: append([]string{"consumer--c1"}, routeGoTags...),
			}).json()))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("inconsistent ownership")))
		})

		It("rejects multiple Go tag hits", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, pageOf(nil,
				(&pluginRecord{id: "a", name: "acl", route: routeID, tags: routeGoTags}).json(),
				(&pluginRecord{id: "b", name: "acl", route: routeID, tags: routeGoTags}).json(),
			))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("found multiple")))
		})
	})

	Describe("malformed inventory", func() {
		DescribeTable("fails without writing or setting the ID",
			func(record, message string) {
				expectRouteID(api)
				expectList(api, routeTags, nil, pageOf(nil, record))

				_, err := client.CreateOrReplacePlugin(ctx, plugin)
				Expect(err).To(MatchError(ContainSubstring(message)))
				Expect(plugin.GetId()).To(BeEmpty())
			},
			Entry("record that is not an object", `"a"`, "failed to decode plugin list"),
			Entry("desired-name record without id", `{"name":"acl"}`, "has no id"),
			Entry("association without id", `{"id":"a","name":"acl","route":{}}`, "association without id"),
			Entry("association that is not an object", `{"id":"a","name":"acl","consumer":"c1"}`, "failed to decode plugin list"),
		)

		It("rejects duplicate IDs", func() {
			record := (&pluginRecord{id: "a", name: "acl", route: routeID, tags: routeGoTags}).json()
			expectRouteID(api)
			expectList(api, routeTags, nil, pageOf(nil, record, record))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("found multiple")))
		})

		DescribeTable("rejects a malformed list response",
			func(body, message string) {
				expectRouteID(api)
				expectList(api, routeTags, nil, body)

				_, err := client.CreateOrReplacePlugin(ctx, plugin)
				Expect(err).To(MatchError(ContainSubstring(message)))
				Expect(plugin.GetId()).To(BeEmpty())
			},
			Entry("not an object", `[]`, "failed to decode plugin list"),
			Entry("missing data", `{}`, "data is missing"),
			Entry("null body", `null`, "data is missing"),
			Entry("null data", `{"data":null}`, "data is missing"),
			Entry("data that is not an array", `{"data":{}}`, "failed to decode plugin list"),
			Entry("offset that is not a string", `{"data":[],"offset":1}`, "failed to decode plugin list"),
		)

		It("rejects a failed list response", func() {
			expectRouteID(api)
			api.EXPECT().ListPluginWithResponse(mock.Anything, mock.Anything).Return(
				&kong.ListPluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError}, Body: []byte(`{}`)}, nil,
			).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("failed to list plugins (500)")))
		})
	})

	Describe("route ID binding", func() {
		var bound *boundTestPlugin

		BeforeEach(func() {
			bound = &boundTestPlugin{testPlugin: *plugin, routeId: routeID, namePrefix: "bond"}
		})

		It("resolves, binds and writes by route ID and adds the realm tag", func() {
			api.EXPECT().GetRouteWithResponse(mock.Anything, routeID).Return(
				&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Route{Id: ptr(routeID)}}, nil,
			).Once()
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, emptyPage)
			api.EXPECT().UpsertPluginForRouteWithResponse(mock.Anything, routeID, mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, _, id string, body kong.UpsertPluginForRouteJSONRequestBody, _ ...kong.RequestEditorFn) (*kong.UpsertPluginForRouteResponse, error) {
					written.id, written.body = id, body
					return &kong.UpsertPluginForRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: echo(id)}, nil
				}).Once()

			_, err := client.CreateOrReplacePlugin(ctx, bound)
			Expect(err).NotTo(HaveOccurred())
			Expect(*written.body.Route).To(Equal(map[string]any{"id": routeID}))
			Expect(*written.body.Tags).To(ConsistOf(append(routeGoTags, "realm--bond")))
		})

		It("does not add the realm tag to a consumer-only plugin", func() {
			bound.route, bound.consumer = nil, ptr("c1")
			expectConsumerID(api, "c1", consumerID)
			expectList(api, consumerTags, nil, emptyPage)
			expectConsumerWrite(api, "c1", &written)

			_, err := client.CreateOrReplacePlugin(ctx, bound)
			Expect(err).NotTo(HaveOccurred())
			Expect(*written.body.Tags).NotTo(ContainElement("realm--bond"))
		})

		It("falls back to the route name without route ID", func() {
			bound.routeId = ""
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, emptyPage)
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, bound)
			Expect(err).NotTo(HaveOccurred())
			Expect(*written.body.Route).To(Equal(map[string]any{"name": ptr("r1")}))
		})
	})

	Describe("pagination", func() {
		It("reads every page before deciding", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, pageOf("o1", (&pluginRecord{id: "x", name: unrelatedName}).json()))
			expectList(api, routeTags, ptr("o1"), pageOf("", (&pluginRecord{
				id: storedID, name: "acl", route: routeID, tags: routeGoTags, protocols: []string{"http"}, config: allowConfig,
			}).json()))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(plugin.GetId()).To(Equal(storedID))
		})

		It("rejects legacy plugins in the same slot spread across pages", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, pageOf("o1", (&pluginRecord{id: "a", name: "acl", route: routeID, tags: legacyRouteTags}).json()))
			expectSlot(api, ptr("o1"), pageOf(nil, (&pluginRecord{id: "b", name: "acl", route: routeID, tags: legacyRouteTags}).json()))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("found multiple")))
			Expect(plugin.GetId()).To(BeEmpty())
		})

		It("rejects a repeated offset", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, pageOf("o1"))
			expectList(api, routeTags, ptr("o1"), pageOf("o1"))

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("offset repeats")))
		})
	})

	Describe("same-ID updates", func() {
		It("changes the ACL allow list of an adopted plugin under the same ID and keeps all tags", func() {
			plugin.id = legacyID
			plugin.config = map[string]any{"allow": []any{"group-a", "group-b"}}
			current := &pluginRecord{
				id: legacyID, name: "acl", route: routeID, protocols: []string{"http", "https"},
				tags: append([]string{legacyTag, "custom-tag"}, routeGoTags...), config: allowConfig,
			}
			expectRouteID(api)
			expectPluginRecord(api, legacyID, current)
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(written.id).To(Equal(legacyID))
			Expect(*written.body.Tags).To(ConsistOf(current.tags))
			Expect((*written.body.Config)["allow"]).To(ConsistOf("group-a", "group-b"))
		})

		It("does not write when config, bindings and all tags match", func() {
			plugin.id = legacyID
			expectRouteID(api)
			expectPluginRecord(api, legacyID, &pluginRecord{
				id: legacyID, name: "acl", route: routeID, protocols: []string{"grpc", "http"}, instanceName: "kept",
				tags: append([]string{legacyTag, "custom-tag"}, routeGoTags...), config: allowConfig,
			})

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(plugin.GetId()).To(Equal(legacyID))
		})

		It("replaces the ACL allow list so that a group can be revoked", func() {
			plugin.id = storedID
			plugin.config = map[string]any{"allow": []any{"group-b"}}
			expectRouteID(api)
			expectPluginRecord(api, storedID, &pluginRecord{
				id: storedID, name: "acl", route: routeID, tags: routeGoTags,
				config: map[string]any{"allow": []any{"group-a", "group-b"}, "hide_groups_header": true},
			})
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(*written.body.Config).To(Equal(map[string]any{"allow": []any{"group-b"}}))
		})

		It("preserves protocols and instance name of a Go-owned bound plugin", func() {
			plugin.id = storedID
			plugin.config = map[string]any{"allow": []any{"group-b"}}
			expectRouteID(api)
			expectPluginRecord(api, storedID, &pluginRecord{
				id: storedID, name: "acl", route: routeID, protocols: []string{"https", "http"}, instanceName: "kept",
				tags: routeGoTags, config: allowConfig,
			})
			expectRouteWrite(api, &written)

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(*written.body.Protocols).To(Equal([]kong.CreatePluginForConsumerRequestProtocols{"http", "https"}))
			Expect(*written.body.InstanceName).To(Equal("kept"))
		})

		It("rejects and does not persist a write response with another ID", func() {
			expectRouteID(api)
			expectList(api, routeTags, nil, emptyPage)
			expectSlot(api, nil, emptyPage)
			api.EXPECT().UpsertPluginForRouteWithResponse(mock.Anything, "r1", mock.Anything, mock.Anything).Return(
				&kong.UpsertPluginForRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: echo("foreign")}, nil,
			).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).To(MatchError(ContainSubstring("returned id foreign")))
			Expect(plugin.GetId()).To(BeEmpty())
		})

		It("falls back to the requested ID when the write response has none", func() {
			plugin.id = storedID
			expectRouteID(api)
			expectPluginRecord(api, storedID, &pluginRecord{id: storedID, name: "acl", route: routeID, tags: routeGoTags})
			api.EXPECT().UpsertPluginForRouteWithResponse(mock.Anything, "r1", storedID, mock.Anything).Return(
				&kong.UpsertPluginForRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: []byte(`{"name":"acl"}`)}, nil,
			).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
			Expect(plugin.GetId()).To(Equal(storedID))
		})
	})
})
