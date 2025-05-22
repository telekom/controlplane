// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"github.com/emirpasic/gods/sets/hashset"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

var ConsumerMatchClaim = "clientId"

var _ client.CustomPlugin = &JwtPlugin{}

type JwtPluginConfig struct {
	ConsumerMatchClaimCustomId  bool         `json:"consumer_match_claim_custom_id,omitempty"`
	ConsumerMatchIgnoreNotFound bool         `json:"consumer_match_ignore_not_found,omitempty"`
	AllowedIss                  *hashset.Set `json:"allowed_iss,omitempty"`
	ConsumerMatch               bool         `json:"consumer_match,omitempty"`
	ConsumerMatchClaim          *string      `json:"consumer_match_claim,omitempty"`
}

type JwtPlugin struct {
	Id     string          `json:"id,omitempty"`
	Config JwtPluginConfig `json:"config,omitempty"`
	route  *gatewayv1.Route
}

func (p *JwtPlugin) GetId() string {
	return p.Id
}

func (p *JwtPlugin) SetId(id string) {
	p.Id = id
	p.route.SetProperty("kongJwtKeycloakPluginId", id)
}

func (p *JwtPlugin) GetName() string {
	return "jwt-keycloak"
}

func (p *JwtPlugin) GetRoute() *string {
	return &p.route.Name
}

func (p *JwtPlugin) GetConsumer() *string {
	return nil
}

func (p *JwtPlugin) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"consumer_match_claim_custom_id":  p.Config.ConsumerMatchClaimCustomId,
		"consumer_match_ignore_not_found": p.Config.ConsumerMatchIgnoreNotFound,
		"allowed_iss":                     p.Config.AllowedIss,
		"consumer_match":                  p.Config.ConsumerMatch,
		"consumer_match_claim":            p.Config.ConsumerMatchClaim,
	}
}

func JwtPluginFromRoute(route *gatewayv1.Route) *JwtPlugin {
	cfg := JwtPluginConfig{
		AllowedIss:                  hashset.New(),
		ConsumerMatchClaimCustomId:  true,
		ConsumerMatchIgnoreNotFound: false,
		ConsumerMatch:               true,
		ConsumerMatchClaim:          &ConsumerMatchClaim,
	}
	for _, downstream := range route.Spec.Downstreams {
		cfg.AllowedIss.Add(downstream.IssuerUrl)
	}

	return &JwtPlugin{
		Id:     route.GetProperty("kongJwtKeycloakPluginId"),
		Config: cfg,
		route:  route,
	}
}
