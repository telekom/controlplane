// Copyright 2025 Deutsche Telekom IT GmbH
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

var _ = Describe("CreateOrReplacePlugin", func() {
	var (
		ctx    context.Context
		plugin *testPlugin
		stored kong.Plugin
		api    *mockclient.MockKongAdminApi
		client clientpkg.KongClient
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		plugin = &testPlugin{id: "plugin-id", name: "acl", config: map[string]any{
			"allow": []any{"group-a"}, "nested": map[string]any{"number": int(1), "enabled": true},
		}}
		stored = kong.Plugin{
			Id: ptr("plugin-id"), Name: ptr("acl"), Enabled: ptr(true), Config: &map[string]any{
				"nested": map[string]any{"enabled": true, "number": float64(1)}, "allow": []any{"group-a"},
			},
			Protocols: ptr([]string{"http"}), Tags: &[]string{"plugin--acl", "consumer--none", "env--test"},
		}
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
	})

	expectPlugin := func() {
		api.EXPECT().GetPluginWithResponse(mock.Anything, "plugin-id").Return(
			&kong.GetPluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &stored}, nil,
		).Once()
	}

	// upsertResponse is what Kong answers a successful write with.
	upsertResponse := &http.Response{StatusCode: http.StatusOK}
	upsertBody := []byte(`{"id":"plugin-id","name":"acl"}`)

	It("does not upsert an equivalent plugin and restores its ID", func() {
		expectPlugin()

		result, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Id).To(Equal(ptr("plugin-id")))
		Expect(plugin.GetId()).To(Equal("plugin-id"))
	})

	It("discovers an equivalent plugin by tags with one list request", func() {
		plugin.id = ""
		api.EXPECT().ListPluginWithResponse(mock.Anything, mock.Anything).Return(
			&kong.ListPluginResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				Body:         []byte(`{"data":[{"id":"plugin-id","name":"acl","enabled":true,"config":{"allow":["group-a"],"nested":{"number":1,"enabled":true}},"protocols":["http"],"tags":["plugin--acl","consumer--none","env--test"]}]}`),
			}, nil,
		).Once()

		result, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Id).To(Equal(ptr("plugin-id")))
		Expect(plugin.GetId()).To(Equal("plugin-id"))
	})

	It("resolves entity references from the ownership tags without extra reads", func() {
		// The mock fails the test if an unexpected call is made, so the absence
		// of route and consumer expectations asserts that no lookup happens.
		plugin.route = ptr("route-name")
		plugin.consumer = ptr("consumer-name")
		stored.Route = &kong.PluginRoute{Id: ptr("route-id")}
		stored.Consumer = &kong.PluginConsumer{Id: ptr("consumer-id")}
		stored.Tags = &[]string{"plugin--acl", "consumer--consumer-name", "env--test", "route--route-name"}
		expectPlugin()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("does not upsert a plugin when tags and protocols are reordered", func() {
		stored.Tags = &[]string{"env--test", "consumer--none", "plugin--acl"}
		expectPlugin()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("does not upsert an equivalent route plugin when Kong returns its ID", func() {
		plugin.route = ptr("route-name")
		stored.Route = &kong.PluginRoute{Id: ptr("89d89d2e-7fbd-4d6d-9133-42f5f9e98f81")}
		stored.Tags = &[]string{"route--route-name", "plugin--acl", "consumer--none", "env--test"}
		expectPlugin()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("does not upsert an equivalent consumer plugin when Kong returns its ID", func() {
		plugin.consumer = ptr("consumer-name")
		stored.Consumer = &kong.PluginConsumer{Id: ptr("d2de9d8c-e035-46f2-9ff5-8308c53a50de")}
		stored.Tags = &[]string{"consumer--consumer-name", "plugin--acl", "env--test"}
		expectPlugin()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("does not upsert an equivalent route-consumer plugin when Kong returns IDs", func() {
		plugin.route = ptr("route-name")
		plugin.consumer = ptr("consumer-name")
		stored.Route = &kong.PluginRoute{Id: ptr("89d89d2e-7fbd-4d6d-9133-42f5f9e98f81")}
		stored.Consumer = &kong.PluginConsumer{Id: ptr("d2de9d8c-e035-46f2-9ff5-8308c53a50de")}
		stored.Tags = &[]string{"route--route-name", "consumer--consumer-name", "plugin--acl", "env--test"}
		expectPlugin()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("does not suppress a changed plugin field",
		func(change func(*kong.Plugin)) {
			change(&stored)
			expectPlugin()
			api.EXPECT().UpsertPluginWithResponse(mock.Anything, "plugin-id", mock.Anything).Return(
				&kong.UpsertPluginResponse{HTTPResponse: upsertResponse, Body: upsertBody}, nil,
			).Once()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("enabled", func(current *kong.Plugin) { current.Enabled = ptr(false) }),
		Entry("config", func(current *kong.Plugin) { (*current.Config)["allow"] = []any{"other"} }),
		Entry("protocols", func(current *kong.Plugin) { current.Protocols = ptr([]string{"https"}) }),
		Entry("tags", func(current *kong.Plugin) { current.Tags = &[]string{"plugin--acl"} }),
		Entry("config key Kong does not report", func(current *kong.Plugin) { delete(*current.Config, "allow") }),
		Entry("value nested under a managed key", func(current *kong.Plugin) {
			(*current.Config)["nested"].(map[string]any)["number"] = float64(2)
		}),
		Entry("key nested under a managed key that Kong does not report", func(current *kong.Plugin) {
			delete((*current.Config)["nested"].(map[string]any), "enabled")
		}),
	)

	It("suppresses the write when Kong fills in config defaults the plugin does not set", func() {
		// Kong answers with the whole plugin schema, so every key the feature
		// leaves unset comes back with its default. Comparing the full answer
		// would write the plugin on every reconciliation.
		(*stored.Config)["deny"] = nil
		(*stored.Config)["hide_groups_header"] = false
		(*stored.Config)["include_consumer_groups"] = false
		(*stored.Config)["nested"].(map[string]any)["timeout"] = float64(60)
		expectPlugin()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("does not sort the configuration of the plugin it returns", func() {
		// Project sorts a copy, so the reconciliation result still carries the
		// configuration exactly as Kong reported it.
		(*stored.Config)["allow"] = []any{"group-b", "group-a"}
		plugin.config["allow"] = []any{"group-a", "group-b"}
		expectPlugin()

		result, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
		Expect((*result.Config)["allow"]).To(Equal([]any{"group-b", "group-a"}))
	})

	It("upserts a route plugin against the route endpoint", func() {
		plugin.route = ptr("route-name")
		stored.Enabled = ptr(false)
		stored.Tags = &[]string{"route--route-name", "plugin--acl", "consumer--none", "env--test"}
		expectPlugin()
		api.EXPECT().UpsertPluginForRouteWithResponse(mock.Anything, "route-name", "plugin-id", mock.Anything).
			RunAndReturn(func(_ context.Context, _, _ string, body kong.UpsertPluginForRouteJSONRequestBody, _ ...kong.RequestEditorFn) (*kong.UpsertPluginForRouteResponse, error) {
				Expect(*body.Name).To(Equal("acl"))
				Expect(*body.Enabled).To(BeTrue())
				Expect(*body.Config).To(HaveKey("allow"))
				return &kong.UpsertPluginForRouteResponse{HTTPResponse: upsertResponse, Body: upsertBody}, nil
			}).Once()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("upserts a consumer plugin against the consumer endpoint", func() {
		plugin.consumer = ptr("consumer-name")
		stored.Enabled = ptr(false)
		stored.Tags = &[]string{"consumer--consumer-name", "plugin--acl", "env--test"}
		expectPlugin()
		api.EXPECT().UpsertPluginForConsumerWithResponse(mock.Anything, "consumer-name", "plugin-id", mock.Anything).Return(
			&kong.UpsertPluginForConsumerResponse{HTTPResponse: upsertResponse, Body: upsertBody}, nil,
		).Once()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns a plugin write failure", func() {
		stored.Enabled = ptr(false)
		expectPlugin()
		api.EXPECT().UpsertPluginWithResponse(mock.Anything, "plugin-id", mock.Anything).
			Return(nil, errors.New("write failed")).Once()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).To(MatchError(ContainSubstring("failed to write plugin")))
	})

	DescribeTable("ignores entity references, which the ownership tags already cover",
		func(change func(*kong.Plugin)) {
			change(&stored)
			expectPlugin()

			_, err := client.CreateOrReplacePlugin(ctx, plugin)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("route", func(current *kong.Plugin) { current.Route = &kong.PluginRoute{Id: ptr("route-id")} }),
		Entry("consumer", func(current *kong.Plugin) { current.Consumer = &kong.PluginConsumer{Id: ptr("consumer-id")} }),
		Entry("service", func(current *kong.Plugin) { current.Service = &kong.PluginService{Id: ptr("service-id")} }),
	)

	It("upserts a plugin missing by stored ID under a fresh ID", func() {
		api.EXPECT().GetPluginWithResponse(mock.Anything, "plugin-id").Return(
			&kong.GetPluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
		).Once()
		api.EXPECT().ListPluginWithResponse(mock.Anything, mock.Anything).Return(
			&kong.ListPluginResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				Body:         []byte(`{"data":[]}`),
			}, nil,
		).Once()
		api.EXPECT().UpsertPluginWithResponse(mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, pluginId string, _ kong.UpsertPluginJSONRequestBody, _ ...kong.RequestEditorFn) (*kong.UpsertPluginResponse, error) {
				Expect(pluginId).NotTo(BeEmpty())
				Expect(pluginId).NotTo(Equal("plugin-id"))
				return &kong.UpsertPluginResponse{HTTPResponse: upsertResponse, Body: upsertBody}, nil
			}).Once()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a successful plugin read without a body", func() {
		api.EXPECT().GetPluginWithResponse(mock.Anything, "plugin-id").Return(
			&kong.GetPluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil,
		).Once()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).To(MatchError(ContainSubstring("plugin response body is missing")))
	})

	It("returns a plugin GET failure without writing", func() {
		api.EXPECT().GetPluginWithResponse(mock.Anything, "plugin-id").Return(nil, errors.New("read failed")).Once()

		_, err := client.CreateOrReplacePlugin(ctx, plugin)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("CleanupPlugins", func() {
	var (
		ctx    context.Context
		route  *v1.Route
		api    *mockclient.MockKongAdminApi
		client clientpkg.KongClient
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		route = &v1.Route{ObjectMeta: metav1.ObjectMeta{Name: "test-route"}}
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
	})

	expectPlugins := func(body string) {
		api.EXPECT().ListPluginWithResponse(mock.Anything, mock.Anything).Return(
			&kong.ListPluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: []byte(body)}, nil,
		).Once()
	}

	It("deletes a plugin that the route no longer declares", func() {
		expectPlugins(`{"data":[{"id":"stale-id","name":"acl"}]}`)
		api.EXPECT().DeletePluginWithResponse(mock.Anything, "stale-id").Return(
			&kong.DeletePluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}, nil,
		).Once()

		Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(Succeed())
	})

	It("keeps a plugin the route still declares", func() {
		expectPlugins(`{"data":[{"id":"plugin-id","name":"acl"}]}`)

		Expect(client.CleanupPlugins(ctx, route, nil, []clientpkg.CustomPlugin{&testPlugin{id: "plugin-id", name: "acl"}})).To(Succeed())
	})

	It("reports a rejected delete instead of reporting success", func() {
		expectPlugins(`{"data":[{"id":"stale-id","name":"acl"}]}`)
		api.EXPECT().DeletePluginWithResponse(mock.Anything, "stale-id").Return(
			&kong.DeletePluginResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
				Body:         []byte(`{"message":"database unavailable"}`),
			}, nil,
		).Once()

		Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(MatchError(ContainSubstring("failed to delete plugin")))
	})

	It("treats an already deleted plugin as cleaned up", func() {
		expectPlugins(`{"data":[{"id":"stale-id","name":"acl"}]}`)
		api.EXPECT().DeletePluginWithResponse(mock.Anything, "stale-id").Return(
			&kong.DeletePluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
		).Once()

		Expect(client.CleanupPlugins(ctx, route, nil, nil)).To(Succeed())
	})

	It("requires a route or a consumer", func() {
		Expect(client.CleanupPlugins(ctx, nil, nil, nil)).To(MatchError(ContainSubstring("either route or consumer must be provided")))
	})
})

var _ = Describe("DeletePlugin", func() {
	It("uses the full ownership tags to find a route plugin", func() {
		ctx := contextutil.WithEnv(context.Background(), "test")
		api := mockclient.NewMockKongAdminApi(GinkgoT())
		client := clientpkg.NewKongClient(api)
		plugin := &testPlugin{name: "acl", route: ptr("route-name")}

		api.EXPECT().ListPluginWithResponse(mock.Anything, mock.MatchedBy(func(params *kong.ListPluginParams) bool {
			return params.Tags != nil && *params.Tags == "env--test,plugin--acl,route--route-name,consumer--none"
		})).Return(&kong.ListPluginResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			Body:         []byte(`{"data":[{"id":"plugin-id"}]}`),
		}, nil)
		api.EXPECT().DeletePluginWithResponse(mock.Anything, "plugin-id").Return(
			&kong.DeletePluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}, nil)

		Expect(client.DeletePlugin(ctx, plugin)).To(Succeed())
	})
})
