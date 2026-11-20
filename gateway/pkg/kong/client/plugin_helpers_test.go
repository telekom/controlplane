// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/stretchr/testify/mock"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	mockclient "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"

	. "github.com/onsi/gomega"
)

// pluginRecord describes a plugin as Kong reports it.
type pluginRecord struct {
	id, name                 string
	route, consumer, service string
	tags                     []string
	protocols                []string
	instanceName             string
	config                   map[string]any
}

func (r *pluginRecord) toMap() map[string]any {
	record := map[string]any{"id": r.id, "name": r.name, "enabled": true}
	if r.route != "" {
		record["route"] = map[string]any{"id": r.route}
	}
	if r.consumer != "" {
		record["consumer"] = map[string]any{"id": r.consumer}
	}
	if r.service != "" {
		record["service"] = map[string]any{"id": r.service}
	}
	if r.tags != nil {
		record["tags"] = r.tags
	}
	if r.protocols != nil {
		record["protocols"] = r.protocols
	}
	if r.instanceName != "" {
		record["instance_name"] = r.instanceName
	}
	if r.config != nil {
		record["config"] = r.config
	}
	return record
}

func (r *pluginRecord) json() string {
	data, err := json.Marshal(r.toMap())
	Expect(err).NotTo(HaveOccurred())
	return string(data)
}

// pageOf renders a plugin list page from raw JSON records.
func pageOf(offset any, records ...string) string {
	page := `{"data":[`
	for i, record := range records {
		if i > 0 {
			page += ","
		}
		page += record
	}
	page += `]`
	if offset != nil {
		data, err := json.Marshal(offset)
		Expect(err).NotTo(HaveOccurred())
		page += `,"offset":` + string(data)
	}
	return page + `}`
}

func listResponse(body string) *kong.ListPluginResponse {
	return &kong.ListPluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: []byte(body)}
}

// expectList expects one plugin list request for exactly the given tag
// filter and page offset.
func expectList(api *mockclient.MockKongAdminApi, tags string, offset *string, body string) {
	api.EXPECT().ListPluginWithResponse(mock.Anything, mock.MatchedBy(func(params *kong.ListPluginParams) bool {
		if params.Tags == nil || *params.Tags != tags || params.Size == nil || *params.Size != 1000 {
			return false
		}
		if offset == nil {
			return params.Offset == nil
		}
		return params.Offset != nil && *params.Offset == *offset
	})).Return(listResponse(body), nil).Once()
}

// expectRouteID expects the lookup of route r1.
func expectRouteID(api *mockclient.MockKongAdminApi) {
	api.EXPECT().GetRouteWithResponse(mock.Anything, "r1").Return(
		&kong.GetRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Route{Id: ptr(routeID)}}, nil,
	).Once()
}

func expectConsumerID(api *mockclient.MockKongAdminApi, name, id string) {
	api.EXPECT().GetConsumerWithResponse(mock.Anything, name).Return(
		&kong.GetConsumerResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &kong.Consumer{Id: &id}}, nil,
	).Once()
}

func expectConsumerStatus(api *mockclient.MockKongAdminApi, name string, status int) {
	api.EXPECT().GetConsumerWithResponse(mock.Anything, name).Return(
		&kong.GetConsumerResponse{HTTPResponse: &http.Response{StatusCode: status}, Body: []byte(`{"message":"x"}`)}, nil,
	).Once()
}

func expectPluginRecord(api *mockclient.MockKongAdminApi, id string, record *pluginRecord) {
	body := record.json()
	var parsed kong.Plugin
	Expect(json.Unmarshal([]byte(body), &parsed)).To(Succeed())
	api.EXPECT().GetPluginWithResponse(mock.Anything, id).Return(
		&kong.GetPluginResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: []byte(body), JSON200: &parsed}, nil,
	).Once()
}

func expectPluginStatus(api *mockclient.MockKongAdminApi, id string, status int) {
	api.EXPECT().GetPluginWithResponse(mock.Anything, id).Return(
		&kong.GetPluginResponse{HTTPResponse: &http.Response{StatusCode: status}, Body: []byte(`{"message":"x"}`)}, nil,
	).Once()
}

// writtenBody captures the body of a plugin write.
type writtenBody struct {
	id   string
	body kong.UpsertPluginJSONRequestBody
}

func echo(id string) []byte {
	return []byte(`{"id":"` + id + `","name":"acl"}`)
}

func expectRouteWrite(api *mockclient.MockKongAdminApi, captured *writtenBody) {
	api.EXPECT().UpsertPluginForRouteWithResponse(mock.Anything, "r1", mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _, id string, body kong.UpsertPluginForRouteJSONRequestBody, _ ...kong.RequestEditorFn) (*kong.UpsertPluginForRouteResponse, error) {
			captured.id, captured.body = id, body
			return &kong.UpsertPluginForRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: echo(id)}, nil
		}).Once()
}

func expectConsumerWrite(api *mockclient.MockKongAdminApi, consumer string, captured *writtenBody) {
	api.EXPECT().UpsertPluginForConsumerWithResponse(mock.Anything, consumer, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _, id string, body kong.UpsertPluginForConsumerJSONRequestBody, _ ...kong.RequestEditorFn) (*kong.UpsertPluginForConsumerResponse, error) {
			captured.id, captured.body = id, body
			return &kong.UpsertPluginForConsumerResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: echo(id)}, nil
		}).Once()
}

func expectDelete(api *mockclient.MockKongAdminApi, id string, status int, deleted *[]string) {
	api.EXPECT().DeletePluginWithResponse(mock.Anything, id).
		RunAndReturn(func(_ context.Context, id string, _ ...kong.RequestEditorFn) (*kong.DeletePluginResponse, error) {
			*deleted = append(*deleted, id)
			return &kong.DeletePluginResponse{HTTPResponse: &http.Response{StatusCode: status}, Body: []byte(`{"message":"x"}`)}, nil
		}).Once()
}

// expectSlot expects one request for a page of the plugins bound to the route with routeID.
func expectSlot(api *mockclient.MockKongAdminApi, offset *string, body string) {
	api.EXPECT().ListPluginsForRouteWithResponse(mock.Anything, routeID, mock.MatchedBy(func(params *kong.ListPluginsForRouteParams) bool {
		if params.Tags != nil || params.Size == nil || *params.Size != 1000 {
			return false
		}
		if offset == nil {
			return params.Offset == nil
		}
		return params.Offset != nil && *params.Offset == *offset
	})).Return(&kong.ListPluginsForRouteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, Body: []byte(body)}, nil).Once()
}

// boundTestPlugin is a testPlugin bound by route ID with a name prefix.
type boundTestPlugin struct {
	testPlugin
	routeId    string
	namePrefix string
}

func (p *boundTestPlugin) GetRouteId() string    { return p.routeId }
func (p *boundTestPlugin) GetNamePrefix() string { return p.namePrefix }
