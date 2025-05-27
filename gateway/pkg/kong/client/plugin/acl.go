// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"

	"github.com/emirpasic/gods/sets/hashset"
)

var _ client.CustomPlugin = &AclPlugin{}

type AclPluginConfig struct {
	Deny             *hashset.Set `json:"deny,omitempty"`
	Allow            *hashset.Set `json:"allow,omitempty"`
	HideGroupsHeader bool         `json:"hide_groups_header,omitempty"`
}

func (c *AclPluginConfig) AddAllow(allow string) {
	c.Allow.Add(allow)
}

func (c *AclPluginConfig) AddDeny(deny string) {
	c.Deny.Add(deny)
}

type AclPlugin struct {
	Id     string          `json:"id,omitempty"`
	Config AclPluginConfig `json:"config,omitempty"`
	route  *gatewayv1.Route
}

func (p *AclPlugin) GetId() string {
	return p.Id
}

func (p *AclPlugin) SetId(id string) {
	p.Id = id
	p.route.SetProperty("kongAclPluginId", id)
}

func (p *AclPlugin) GetName() string {
	return "acl"
}

func (p *AclPlugin) GetRoute() *string {
	return &p.route.Name
}

func (p *AclPlugin) GetConsumer() *string {
	return nil
}

func (p *AclPlugin) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"deny":               p.Config.Deny,
		"allow":              p.Config.Allow,
		"hide_groups_header": p.Config.HideGroupsHeader,
	}
}

func AclPluginFromRoute(route *gatewayv1.Route) *AclPlugin {
	return &AclPlugin{
		Id: route.GetProperty("kongAclPluginId"),
		Config: AclPluginConfig{
			Allow:            hashset.New(),
			Deny:             hashset.New(),
			HideGroupsHeader: false,
		},
		route: route,
	}
}
