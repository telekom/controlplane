// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"github.com/emirpasic/gods/sets/hashset"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

var _ client.CustomPlugin = &IpRestrictionPlugin{}

type IpRestrictionPluginConfig struct {
	Deny    *hashset.Set `json:"deny,omitempty"`
	Allow   *hashset.Set `json:"allow,omitempty"`
	Message string       `json:"message,omizero"`
	Status  int          `json:"status,omizero"`
}

func (c *IpRestrictionPluginConfig) AddAllow(allow string) {
	c.Allow.Add(allow)
}

func (c *IpRestrictionPluginConfig) AddDeny(deny string) {
	c.Deny.Add(deny)
}

type IpRestrictionPlugin struct {
	Id       string                    `json:"id,omitempty"`
	Config   IpRestrictionPluginConfig `json:"config,omitempty"`
	route    *gatewayv1.Route
	consumer *gatewayv1.Consumer
}

func (p *IpRestrictionPlugin) GetId() string {
	return p.Id
}

func (p *IpRestrictionPlugin) SetId(id string) {
	p.Id = id
	if p.consumer != nil {
		p.consumer.SetProperty("kongIpRestrictionPluginId", id)
	}
}

func (p *IpRestrictionPlugin) GetName() string {
	return "ip-restriction"
}

func (p *IpRestrictionPlugin) GetRoute() *string {
	// Currently this plugin is only allowed on Consumers
	return nil
}

func (p *IpRestrictionPlugin) GetConsumer() *string {
	return &p.consumer.Spec.Name
}

func (p *IpRestrictionPlugin) GetConfig() map[string]interface{} {
	cfg := map[string]any{
		"deny":  p.Config.Deny,
		"allow": p.Config.Allow,
	}
	if p.Config.Message != "" {
		cfg["message"] = p.Config.Message
	}
	if p.Config.Status != 0 {
		cfg["status"] = p.Config.Status
	}
	return cfg
}

func IpRestrictionPluginFromConsumer(consumer *gatewayv1.Consumer) *IpRestrictionPlugin {
	return &IpRestrictionPlugin{
		Id: consumer.GetProperty("kongIpRestrictionPluginId"),
		Config: IpRestrictionPluginConfig{
			Allow: hashset.New(),
			Deny:  hashset.New(),
		},
		consumer: consumer,
	}
}
