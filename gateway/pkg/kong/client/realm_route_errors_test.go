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

var _ = Describe("Realm route discovery failures", func() {
	var (
		ctx    context.Context
		api    *mockclient.MockKongAdminApi
		client clientpkg.KongClient
		route  *v1.Route
	)

	notFound := &kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}
	upstream := clientpkg.NewUpstreamOrDie("http://localhost:8080/proxy")

	listRoutes := func(status int, body string) *kong.ListRouteResponse {
		return &kong.ListRouteResponse{HTTPResponse: &http.Response{StatusCode: status}, Body: []byte(body)}
	}

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "bond")
		api = mockclient.NewMockKongAdminApi(GinkgoT())
		client = clientpkg.NewKongClient(api)
		route = &v1.Route{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
		route.SetNamePrefix("bond")
	})

	It("fails if the stored ID lookup fails", func() {
		route.SetRouteId("id")
		api.EXPECT().GetRouteWithResponse(mock.Anything, "id").Return(nil, errors.New("boom")).Once()
		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("looking up route by id")))
	})

	It("fails if the name lookup fails", func() {
		api.EXPECT().GetRouteWithResponse(mock.Anything, "bond--r").Return(nil, errors.New("boom")).Once()
		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("looking up route by name")))
	})

	DescribeTable("fails if the tag lookup fails",
		func(response *kong.ListRouteResponse, err error, message string) {
			api.EXPECT().GetRouteWithResponse(mock.Anything, "bond--r").Return(notFound, nil).Once()
			api.EXPECT().ListRouteWithResponse(mock.Anything, mock.MatchedBy(func(p *kong.ListRouteParams) bool {
				return *p.Tags == "env--bond,route--r" && *p.Size == 1000
			})).Return(response, err).Once()
			Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring(message)))
		},
		Entry("transport", nil, errors.New("boom"), "failed to list routes: boom"),
		Entry("status", listRoutes(http.StatusInternalServerError, `{}`), nil, "failed to list routes (500)"),
		Entry("malformed", listRoutes(http.StatusOK, `[]`), nil, "failed to decode routes list"),
		Entry("missing data", listRoutes(http.StatusOK, `{}`), nil, "response data is missing"),
	)

	It("fails on a repeated list offset", func() {
		api.EXPECT().GetRouteWithResponse(mock.Anything, "bond--r").Return(notFound, nil).Once()
		api.EXPECT().ListRouteWithResponse(mock.Anything, mock.Anything).Return(listRoutes(http.StatusOK, `{"data":[],"offset":"o"}`), nil).Twice()
		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("offset repeats")))
	})

	It("follows list pagination", func() {
		api.EXPECT().GetRouteWithResponse(mock.Anything, "bond--r").Return(notFound, nil).Once()
		api.EXPECT().ListRouteWithResponse(mock.Anything, mock.MatchedBy(func(p *kong.ListRouteParams) bool { return p.Offset == nil })).
			Return(listRoutes(http.StatusOK, `{"data":[{"id":"a","name":"x--r"}],"offset":"o"}`), nil).Once()
		api.EXPECT().ListRouteWithResponse(mock.Anything, mock.MatchedBy(func(p *kong.ListRouteParams) bool { return p.Offset != nil && *p.Offset == "o" })).
			Return(listRoutes(http.StatusOK, `{"data":[{"id":"b","name":"y--r"}]}`), nil).Once()
		err := client.CreateOrReplaceRoute(ctx, route, upstream)
		Expect(err).To(MatchError(ContainSubstring("found 2 route entities")))
	})

	It("fails if the conflict check fails", func() {
		route.SetRouteId("id")
		api.EXPECT().GetRouteWithResponse(mock.Anything, "id").Return(
			&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Route{Id: ptr("id"), Name: ptr("old")}}, nil,
		).Once()
		api.EXPECT().GetRouteWithResponse(mock.Anything, "bond--r").Return(nil, errors.New("boom")).Once()
		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("checking route name conflicts")))
	})

	It("reports a failed write by ID", func() {
		route.SetRouteId("rid")
		route.SetServiceId("sid")
		api.EXPECT().GetRouteWithResponse(mock.Anything, "rid").Return(
			&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Route{Id: ptr("rid"), Name: ptr("bond--r")}}, nil,
		).Once()
		api.EXPECT().GetServiceWithResponse(mock.Anything, "sid").Return(
			&kong.GetServiceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Service{Id: ptr("sid"), Name: ptr("bond--r")}}, nil,
		).Once()
		api.EXPECT().UpsertServiceWithResponse(mock.Anything, "sid", mock.Anything).Return(nil, errors.New("boom")).Once()
		Expect(client.CreateOrReplaceRoute(ctx, route, upstream)).To(MatchError(ContainSubstring("writing located service")))
	})

	It("reports a failed delete by ID", func() {
		route.SetRouteId("rid")
		api.EXPECT().GetRouteWithResponse(mock.Anything, "rid").Return(
			&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Route{Id: ptr("rid"), Name: ptr("bond--r")}}, nil,
		).Once()
		api.EXPECT().GetServiceWithResponse(mock.Anything, "bond--r").Return(&kong.GetServiceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil).Once()
		api.EXPECT().GetServiceWithResponse(mock.Anything, "r").Return(&kong.GetServiceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil).Once()
		api.EXPECT().ListServiceWithResponse(mock.Anything, mock.Anything).Return(
			&kong.ListServiceResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: []byte(`{"data":[]}`)}, nil,
		).Once()
		api.EXPECT().GetUpstreamWithResponse(mock.Anything, mock.Anything).Return(&kong.GetUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}}, nil).Twice()
		api.EXPECT().ListUpstreamWithResponse(mock.Anything, mock.Anything).Return(
			&kong.ListUpstreamResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: []byte(`{"data":[]}`)}, nil,
		).Once()
		api.EXPECT().DeleteRouteWithResponse(mock.Anything, "rid").Return(
			&kong.DeleteRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError}, Body: []byte(`{}`)}, nil,
		).Once()
		Expect(client.DeleteRoute(ctx, route)).To(MatchError(ContainSubstring("failed to delete route (500)")))
	})
})
