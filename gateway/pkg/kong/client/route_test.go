// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
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

var _ = Describe("CreateOrReplaceRoute", func() {
	var (
		ctx      context.Context
		route    *v1.Route
		upstream clientpkg.Upstream
		api      *mockclient.MockKongAdminApi
		client   clientpkg.KongClient
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test")
		route = &v1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "test-route"},
			Spec: v1.RouteSpec{
				Hostnames: []string{"api.example", "api.example.org"},
				Paths:     []string{"/v1", "/v2"},
			},
		}
		upstream = &clientpkg.CustomUpstream{Scheme: "https", Host: "upstream.example", Port: 443, Path: "/api"}
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
	})

	matchingService := func() *kong.GetServiceResponse {
		return &kong.GetServiceResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &kong.Service{
				Id: ptr("service-id"), Name: ptr("test-route"), Enabled: ptr(true),
				Host: ptr("upstream.example"), Path: ptr("/api"), Port: ptr(443),
				Protocol: ptr("https"), Tags: &[]string{"route--test-route", "env--test"},
			},
		}
	}

	matchingRoute := func() *kong.GetRouteResponse {
		return &kong.GetRouteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &kong.Route{
				Id: ptr("route-id"), Name: ptr("test-route"),
				Protocols:        ptr([]string{"https", "http"}),
				Paths:            ptr([]string{"/v1", "/v2"}),
				Hosts:            ptr([]string{"api.example.org", "api.example"}),
				Service:          &kong.RouteService{Id: ptr("service-id")},
				RequestBuffering: ptr(true), ResponseBuffering: ptr(true),
				HttpsRedirectStatusCode: ptr(426),
				Tags:                    &[]string{"route--test-route", "env--test"},
			},
		}
	}

	expectMatchingRoute := func() {
		api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(matchingRoute(), nil)
	}

	It("does not upsert a matching service", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
		expectMatchingRoute()

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
		Expect(route.GetProperty("serviceId")).To(Equal("service-id"))
	})

	It("does not upsert a service when tags are reordered", func() {
		response := matchingService()
		response.JSON200.Tags = &[]string{"env--test", "route--test-route"}
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(response, nil)
		expectMatchingRoute()

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
	})

	It("upserts a missing service", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(
			&kong.GetServiceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
		)
		api.EXPECT().UpsertServiceWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.UpsertServiceResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.Service{Id: ptr("service-id")},
			}, nil,
		)
		expectMatchingRoute()

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
	})

	It("upserts a changed service", func() {
		response := matchingService()
		response.JSON200.Host = ptr("old.example")
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(response, nil)
		api.EXPECT().UpsertServiceWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.UpsertServiceResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.Service{Id: ptr("service-id")},
			}, nil,
		)
		expectMatchingRoute()

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
	})

	It("rejects an unexpected service read status without writing", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(
			&kong.GetServiceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError}}, nil,
		)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("failed to get service (500)")))
	})

	It("rejects a successful service read without a body", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(
			&kong.GetServiceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil,
		)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("service response body is missing")))
	})

	It("rejects a successful service read without an ID", func() {
		response := matchingService()
		response.JSON200.Id = nil
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(response, nil)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("service response ID is missing")))
	})

	It("does not upsert a matching route and restores IDs", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
		api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(matchingRoute(), nil)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
		Expect(route.GetProperty("serviceId")).To(Equal("service-id"))
		Expect(route.GetProperty("routeId")).To(Equal("route-id"))
	})

	It("does not upsert a route when set-like fields are reordered", func() {
		response := matchingRoute()
		response.JSON200.Protocols = ptr([]string{"http", "https"})
		response.JSON200.Hosts = ptr([]string{"api.example", "api.example.org"})
		response.JSON200.Tags = &[]string{"env--test", "route--test-route"}
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
		api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(response, nil)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
	})

	DescribeTable("upserts a changed route",
		func(change func(*kong.Route)) {
			response := matchingRoute()
			change(response.JSON200)
			api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
			api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(response, nil)
			api.EXPECT().UpsertRouteWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(body kong.CreateRouteJSONRequestBody) bool {
				return body.Methods == nil
			})).Return(
				&kong.UpsertRouteResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusOK},
					JSON200:      &kong.Route{Id: ptr("route-id")},
				}, nil,
			)

			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
		},
		Entry("when hosts change", func(current *kong.Route) { current.Hosts = ptr([]string{"other.example"}) }),
		Entry("when path order changes", func(current *kong.Route) { current.Paths = ptr([]string{"/v2", "/v1"}) }),
		Entry("when request buffering changes", func(current *kong.Route) { current.RequestBuffering = ptr(false) }),
		Entry("when a non-health route has a method restriction", func(current *kong.Route) { current.Methods = ptr([]string{http.MethodHead}) }),
	)

	DescribeTable("configures methods from the healthProbe flag",
		func(upstreamPath string, health bool, expectedMethods *[]string) {
			route.Spec.Traffic.HealthProbe = health
			upstream = &clientpkg.CustomUpstream{Scheme: "https", Host: "upstream.example", Port: 443, Path: upstreamPath}
			service := matchingService()
			service.JSON200.Path = ptr(upstreamPath)
			api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(service, nil)
			api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(
				&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
			)
			api.EXPECT().UpsertRouteWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(body kong.CreateRouteJSONRequestBody) bool {
				Expect(body.Methods).To(Equal(expectedMethods))
				return true
			})).Return(&kong.UpsertRouteResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.Route{Id: ptr("route-id")},
			}, nil)
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
		},
		Entry("HEAD for a health probe with arbitrary upstream", "/api", true, ptr([]string{http.MethodHead})),
		Entry("no restriction for ordinary routes", "/api", false, nil),
		Entry("no restriction for the old placeholder path", "/api/v1/zone-health", false, nil),
	)

	DescribeTable("reconciles zone-health method drift",
		func(currentMethods *[]string, shouldWrite bool) {
			route.Spec.Traffic.HealthProbe = true
			upstream = &clientpkg.CustomUpstream{Scheme: "https", Host: "upstream.example", Port: 443, Path: "/api"}
			service := matchingService()
			service.JSON200.Path = ptr("/api")
			current := matchingRoute()
			current.JSON200.Methods = currentMethods
			api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(service, nil)
			api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(current, nil)
			if shouldWrite {
				api.EXPECT().UpsertRouteWithResponse(mock.Anything, "test-route", mock.MatchedBy(func(body kong.CreateRouteJSONRequestBody) bool {
					return body.Methods != nil && len(*body.Methods) == 1 && (*body.Methods)[0] == http.MethodHead
				})).Return(&kong.UpsertRouteResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusOK},
					JSON200:      &kong.Route{Id: ptr("route-id")},
				}, nil)
			}
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
		},
		Entry("writes a missing restriction", nil, true),
		Entry("corrects extra methods", ptr([]string{http.MethodHead, http.MethodGet}), true),
		Entry("does not write a matching restriction", ptr([]string{http.MethodHead}), false),
	)

	It("upserts a missing route", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
		api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(
			&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil,
		)
		api.EXPECT().UpsertRouteWithResponse(mock.Anything, "test-route", mock.Anything).Return(
			&kong.UpsertRouteResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &kong.Route{Id: ptr("route-id")},
			}, nil,
		)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(Succeed())
	})

	It("rejects a successful route read without a body", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
		api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(
			&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil,
		)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("route response body is missing")))
	})

	It("rejects an unexpected route read status without writing", func() {
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
		api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(
			&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError}}, nil,
		)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("failed to get route (500)")))
	})

	It("rejects a successful route read without an ID", func() {
		response := matchingRoute()
		response.JSON200.Id = nil
		api.EXPECT().GetServiceWithResponse(mock.Anything, "test-route").Return(matchingService(), nil)
		api.EXPECT().GetRouteWithResponse(mock.Anything, "test-route").Return(response, nil)

		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("route response ID is missing")))
	})
})
