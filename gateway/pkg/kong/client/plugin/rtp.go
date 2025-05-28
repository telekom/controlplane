// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"github.com/emirpasic/gods/sets/hashset"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

const JumperConfigKey = "jumper_config"

var _ client.CustomPlugin = &RequestTransformerPlugin{}

// RtPluginRecord all records must match the format "key:value"
// To ensure this format, we use a dedicated datastructure that
// json encodes to an array of strings
type RtPluginRecord struct {
	Headers     *StringMap `json:"headers,omitempty"`
	Body        *StringMap `json:"body,omitempty"`
	Querystring *StringMap `json:"querystring,omitempty"`
}

func (r *RtPluginRecord) AddHeader(key, value string) *RtPluginRecord {
	if r.Headers == nil {
		r.Headers = New()
	}
	r.Headers.AddKV(key, value)
	return r
}

func (r *RtPluginRecord) AddBody(key, value string) *RtPluginRecord {
	if r.Body == nil {
		r.Body = New()
	}
	r.Body.AddKV(key, value)
	return r
}

func (r *RtPluginRecord) AddQuerystring(key, value string) *RtPluginRecord {
	if r.Querystring == nil {
		r.Querystring = New()
	}
	r.Querystring.AddKV(key, value)
	return r
}

// RtPluginRemoveRecord all records must match the format "key"
// Therefore, we can use a set
type RtPluginRemoveRecord struct {
	Headers     *hashset.Set `json:"headers,omitempty"`
	Body        *hashset.Set `json:"body,omitempty"`
	Querystring *hashset.Set `json:"querystring,omitempty"`
}

func (r *RtPluginRemoveRecord) AddHeader(value string) *RtPluginRemoveRecord {
	if r.Headers == nil {
		r.Headers = hashset.New()
	}
	r.Headers.Add(value)
	return r
}

func (r *RtPluginRemoveRecord) AddBody(value string) *RtPluginRemoveRecord {
	if r.Body == nil {
		r.Body = hashset.New()
	}
	r.Body.Add(value)
	return r
}

func (r *RtPluginRemoveRecord) AddQuerystring(value string) *RtPluginRemoveRecord {
	if r.Querystring == nil {
		r.Querystring = hashset.New()
	}
	r.Querystring.Add(value)
	return r
}

type RequestTransformerPluginConfig struct {
	Append  RtPluginRecord       `json:"append,omitempty"`
	Rename  RtPluginRecord       `json:"rename,omitempty"`
	Replace RtPluginRecord       `json:"replace,omitempty"`
	Remove  RtPluginRemoveRecord `json:"remove,omitempty"`
	Add     RtPluginRecord       `json:"add,omitempty"`
}

type RequestTransformerPlugin struct {
	Id     string                         `json:"id,omitempty"`
	Config RequestTransformerPluginConfig `json:"config,omitempty"`
	route  *gatewayv1.Route
}

func (p *RequestTransformerPlugin) GetId() string {
	return p.Id
}

func (p *RequestTransformerPlugin) SetId(id string) {
	p.Id = id
	p.route.SetProperty("kongRequestTransformerPluginId", id)
}

func (p *RequestTransformerPlugin) GetName() string {
	return "request-transformer"
}

func (p *RequestTransformerPlugin) GetRoute() *string {
	return &p.route.Name
}

func (p *RequestTransformerPlugin) GetConsumer() *string {
	return nil
}

func (p *RequestTransformerPlugin) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"append":  p.Config.Append,
		"rename":  p.Config.Rename,
		"replace": p.Config.Replace,
		"remove":  p.Config.Remove,
		"add":     p.Config.Add,
	}
}

func RequestTransformerPluginFromRoute(route *gatewayv1.Route) *RequestTransformerPlugin {

	return &RequestTransformerPlugin{
		Id:     route.GetProperty("kongRequestTransformerPluginId"),
		Config: RequestTransformerPluginConfig{},
		route:  route,
	}
}
