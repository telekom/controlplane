// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

var _ client.CustomPlugin = &RequestTerminationPlugin{}

const requestTerminationPluginIdKey = "kongRequestTerminationPluginId"

type RequestTerminationPluginConfig struct {
	StatusCode int `json:"status_code"`
}

// RequestTerminationPlugin configures the Kong request-termination plugin,
// which answers every matching request directly without calling the upstream.
type RequestTerminationPlugin struct {
	Id     string                         `json:"id,omitempty"`
	Config RequestTerminationPluginConfig `json:"config,omitempty"`
	route  *gatewayv1.Route
}

func (p *RequestTerminationPlugin) GetId() string {
	return p.Id
}

func (p *RequestTerminationPlugin) SetId(id string) {
	p.Id = id
	p.route.SetProperty(requestTerminationPluginIdKey, id)
}

func (p *RequestTerminationPlugin) GetName() string {
	return "request-termination"
}

func (p *RequestTerminationPlugin) GetRoute() *string {
	return &p.route.Name
}

func (p *RequestTerminationPlugin) GetConsumer() *string {
	return nil
}

func (p *RequestTerminationPlugin) GetConfig() map[string]any {
	return map[string]any{
		"status_code": p.Config.StatusCode,
	}
}

func RequestTerminationPluginFromRoute(route *gatewayv1.Route) *RequestTerminationPlugin {
	return &RequestTerminationPlugin{
		Id:    route.GetProperty(requestTerminationPluginIdKey),
		route: route,
	}
}
