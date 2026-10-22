// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type pluginResponseTransport func(*http.Request) (*http.Response, error)

func (f pluginResponseTransport) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

var _ = Describe("Plugin response decoding regression", func() {
	DescribeTable("accepts object bindings after a successful consumer plugin write", func(responseBody string, expected kong.Plugin) {
		var requests []string
		api, err := kong.NewClientWithResponses("http://kong.invalid", kong.WithHTTPClient(pluginResponseTransport(func(request *http.Request) (*http.Response, error) {
			key := request.Method + " " + request.URL.Path
			requests = append(requests, key)
			var body string
			switch key {
			case "GET /plugins/plugin-id":
				body = `{"id":"plugin-id","name":"rate-limiting","enabled":false}`
			case "GET /consumers/consumer-name":
				body = `{"id":"consumer-id"}`
			case "GET /routes/route-name":
				body = `{"id":"route-id"}`
			case "PUT /consumers/consumer-name/plugins/plugin-id":
				defer func() { Expect(request.Body.Close()).To(Succeed()) }()
				var payload map[string]any
				Expect(json.NewDecoder(request.Body).Decode(&payload)).To(Succeed())
				Expect(payload).To(HaveKeyWithValue("route", map[string]any{"name": "route-name"}))
				Expect(payload).To(HaveKeyWithValue("consumer", "consumer-name"))
				body = responseBody
			default:
				Fail("unexpected HTTP request: " + key)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    request,
			}, nil
		})))
		Expect(err).NotTo(HaveOccurred())
		plugin := &testPlugin{
			id: "plugin-id", name: "rate-limiting", route: ptr("route-name"), consumer: ptr("consumer-name"),
			config: map[string]any{"minute": 10, "policy": "local"},
		}

		written, err := clientpkg.NewKongClient(api).CreateOrReplacePlugin(contextutil.WithEnv(context.Background(), "test"), plugin)
		Expect(requests).To(Equal([]string{
			"GET /plugins/plugin-id",
			"GET /consumers/consumer-name",
			"GET /routes/route-name",
			"PUT /consumers/consumer-name/plugins/plugin-id",
		}))
		Expect(err).NotTo(HaveOccurred())
		Expect(written).To(Equal(&expected))
		Expect(plugin.GetId()).To(Equal("plugin-id"))
	},
		Entry("full response", `{"id":"plugin-id","name":"rate-limiting","enabled":true,"config":{"minute":10,"policy":"local"},"route":{"id":"route-id"},"consumer":{"id":"consumer-id"},"service":null}`, kong.Plugin{
			Id: ptr("plugin-id"), Name: ptr("rate-limiting"), Enabled: ptr(true),
			Config: &map[string]any{"minute": float64(10), "policy": "local"},
			Route:  &kong.PluginRoute{Id: ptr("route-id")}, Consumer: &kong.PluginConsumer{Id: ptr("consumer-id")},
		}),
		Entry("minimal response", `{"route":{}}`, kong.Plugin{Id: ptr("plugin-id"), Route: &kong.PluginRoute{}}),
		Entry("null route control", `{"route":null}`, kong.Plugin{Id: ptr("plugin-id")}),
		Entry("consumer object", `{"consumer":{"id":"consumer-id"}}`, kong.Plugin{Id: ptr("plugin-id"), Consumer: &kong.PluginConsumer{Id: ptr("consumer-id")}}),
		Entry("service object", `{"service":{"id":"service-id"}}`, kong.Plugin{Id: ptr("plugin-id"), Service: &kong.PluginService{Id: ptr("service-id")}}),
	)

	DescribeTable("preserves bindings and arbitrary config in other upsert responses", func(parse func(*http.Response) (*kong.Plugin, error)) {
		response := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"plugin-id","route":{"id":"route-id"},"consumer":{"id":"consumer-id"},"service":{"id":"service-id"},"config":{"policy":"local","nested":{"enabled":true}}}`)),
		}
		plugin, err := parse(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(plugin.Route.Id).To(Equal(ptr("route-id")))
		Expect(plugin.Consumer.Id).To(Equal(ptr("consumer-id")))
		Expect(plugin.Service.Id).To(Equal(ptr("service-id")))
		Expect(*plugin.Config).To(Equal(map[string]any{"policy": "local", "nested": map[string]any{"enabled": true}}))
	},
		Entry("global", func(response *http.Response) (*kong.Plugin, error) {
			parsed, err := kong.ParseUpsertPluginResponse(response)
			if err != nil {
				return nil, err
			}
			return parsed.JSON200, nil
		}),
		Entry("route", func(response *http.Response) (*kong.Plugin, error) {
			parsed, err := kong.ParseUpsertPluginForRouteResponse(response)
			if err != nil {
				return nil, err
			}
			return parsed.JSON200, nil
		}),
		Entry("service", func(response *http.Response) (*kong.Plugin, error) {
			parsed, err := kong.ParseUpsertPluginForAServiceResponse(response)
			if err != nil {
				return nil, err
			}
			return parsed.JSON200, nil
		}),
		Entry("consumer PATCH", func(response *http.Response) (*kong.Plugin, error) {
			parsed, err := kong.ParseUpdatePluginForConsumerResponse(response)
			if err != nil {
				return nil, err
			}
			return parsed.JSON200, nil
		}),
	)

	DescribeTable("types consumer PUT and PATCH responses as single plugins", func(parse func(*http.Response) (*kong.Plugin, error), body string, expected kong.Plugin) {
		response := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}
		plugin, err := parse(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(plugin).To(Equal(&expected))
	},
		Entry("PUT with object bindings", parseConsumerPut,
			`{"id":"plugin-id","route":{"id":"route-id"},"consumer":{"id":"consumer-id"},"service":{"id":"service-id"},"config":{"policy":"local","limits":{"minute":10,"hour":null},"headers":["a","b"]}}`,
			kong.Plugin{
				Id: ptr("plugin-id"), Route: &kong.PluginRoute{Id: ptr("route-id")},
				Consumer: &kong.PluginConsumer{Id: ptr("consumer-id")}, Service: &kong.PluginService{Id: ptr("service-id")},
				Config: &map[string]any{"policy": "local", "limits": map[string]any{"minute": float64(10), "hour": nil}, "headers": []any{"a", "b"}},
			}),
		Entry("PUT with null bindings", parseConsumerPut,
			`{"id":"plugin-id","route":null,"consumer":{"id":"consumer-id"},"service":null,"config":{"nested":{"enabled":true}}}`,
			kong.Plugin{
				Id: ptr("plugin-id"), Consumer: &kong.PluginConsumer{Id: ptr("consumer-id")},
				Config: &map[string]any{"nested": map[string]any{"enabled": true}},
			}),
		Entry("PATCH with object bindings", parseConsumerPatch,
			`{"id":"plugin-id","route":{"id":"route-id"},"consumer":{"id":"consumer-id"},"service":{"id":"service-id"},"config":{"policy":"local","limits":{"minute":10,"hour":null},"headers":["a","b"]}}`,
			kong.Plugin{
				Id: ptr("plugin-id"), Route: &kong.PluginRoute{Id: ptr("route-id")},
				Consumer: &kong.PluginConsumer{Id: ptr("consumer-id")}, Service: &kong.PluginService{Id: ptr("service-id")},
				Config: &map[string]any{"policy": "local", "limits": map[string]any{"minute": float64(10), "hour": nil}, "headers": []any{"a", "b"}},
			}),
		Entry("PATCH with null bindings", parseConsumerPatch,
			`{"id":"plugin-id","route":null,"consumer":{"id":"consumer-id"},"service":null,"config":{"nested":{"enabled":true}}}`,
			kong.Plugin{
				Id: ptr("plugin-id"), Consumer: &kong.PluginConsumer{Id: ptr("consumer-id")},
				Config: &map[string]any{"nested": map[string]any{"enabled": true}},
			}),
	)
})

func parseConsumerPut(response *http.Response) (*kong.Plugin, error) {
	parsed, err := kong.ParseUpsertPluginForConsumerResponse(response)
	if err != nil {
		return nil, err
	}
	return parsed.JSON200, nil
}

func parseConsumerPatch(response *http.Response) (*kong.Plugin, error) {
	parsed, err := kong.ParseUpdatePluginForConsumerResponse(response)
	if err != nil {
		return nil, err
	}
	return parsed.JSON200, nil
}
