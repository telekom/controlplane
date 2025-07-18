// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/url"
	"strconv"
)

// CustomPlugin is an abstract interface of a Kong-Plugin
// It must define a `Config: any` field, which is specific to the plugin
type CustomPlugin interface {
	GetId() string
	SetId(string)
	GetName() string
	GetRoute() *string
	GetConsumer() *string
	GetConfig() map[string]any
}

// CustomRoute is an abstract interface of **both** a Kong-Route and Kong-Service
// It must be used in combination with atleast 1 Upstream to be useful
type CustomRoute interface {
	SetRouteId(string)
	SetServiceId(string)
	GetName() string
	GetHost() string
	GetPath() string
}

type CustomConsumer interface {
	SetId(string)
	GetConsumerName() string
}

type Upstream interface {
	GetScheme() string
	GetHost() string
	GetPort() int
	GetPath() string
}

type CustomUpstream struct {
	Scheme string
	Host   string
	Port   int
	Path   string
}

func NewUpstream(rawUrl string) (Upstream, error) {
	url, err := url.Parse(rawUrl)
	if err != nil {
		return nil, err
	}
	return FromURL(url)
}

func NewUpstreamOrDie(rawUrl string) Upstream {
	url, err := url.Parse(rawUrl)
	if err != nil {
		panic(err)
	}
	upstream, err := FromURL(url)
	if err != nil {
		panic(err)
	}
	return upstream
}

func FromURL(url *url.URL) (Upstream, error) {
	intPort, err := strconv.Atoi(url.Port())
	return &CustomUpstream{
		Scheme: url.Scheme,
		Host:   url.Hostname(),
		Port:   intPort,
		Path:   url.Path,
	}, err
}

func (u *CustomUpstream) GetScheme() string {
	return u.Scheme
}

func (u *CustomUpstream) GetHost() string {
	return u.Host
}

func (u *CustomUpstream) GetPort() int {
	return u.Port
}

func (u *CustomUpstream) GetPath() string {
	return u.Path
}
