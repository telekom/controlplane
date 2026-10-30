// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/url"
	"strconv"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
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
	SetUpstreamId(string)
	SetTargetsId(string)
	GetTargetsId() string
	GetName() string
	GetHostnames() []string
	GetAdditionalTags() []string
	GetPaths() []string
	GetRequestBuffering() bool
	GetResponseBuffering() bool
	IsHealthProbe() bool
	IsPassThrough() bool
}

// StoredIds are the Kong IDs of a route, its service and its upstream as recorded before
// the current reconciliation.
type StoredIds = gatewayv1.StoredIds

// RealmRoute is an optional interface of a CustomRoute. If it returns a name prefix, the
// Kong entities of the route are named GetKongName() and discovered, migrated and written
// by ID. Otherwise they are named GetName() as before.
type RealmRoute interface {
	GetNamePrefix() string
	GetKongName() string
	GetLabels() map[string]string
	GetStoredIds() StoredIds
}

// LegacyNameRecorder is an optional interface of a CustomRoute. It records the legacy Kong
// name the route was adopted under.
type LegacyNameRecorder interface {
	SetLegacyName(string)
}

// RouteIdBinder is an optional interface of a CustomPlugin and a CustomRoute. If it returns
// an ID, the plugin is bound to and its parent route is resolved by that Kong route ID.
// Otherwise the route is resolved by name.
type RouteIdBinder interface {
	GetRouteId() string
}

// RealmPlugin is an optional interface of a route-bound CustomPlugin. If it returns a name
// prefix, the plugin is tagged with realm--<prefix>.
type RealmPlugin interface {
	GetNamePrefix() string
}

// CircuitBreakerUpstream points the service to a Kong upstream. CreateOrReplaceRoute writes
// that upstream and its target after the route and service are discovered and before the
// service is written.
type CircuitBreakerUpstream struct {
	CustomUpstream
	Upstream *kong.CreateUpstreamJSONRequestBody
	Target   *kong.CreateTargetForUpstreamJSONRequestBody
}

// UpstreamRemoval wraps the upstream the service points to. CreateOrReplaceRoute deletes the
// Kong upstream of the route after the service is written, so that the service never points
// to a deleted upstream.
type UpstreamRemoval struct {
	Upstream
}

type CustomConsumer interface {
	SetId(string)
	GetConsumerName() string
}

type Upstream interface {
	GetScheme() string
	GetHostname() string
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
	parsedURL, err := url.Parse(rawUrl)
	if err != nil {
		return nil, err
	}
	return FromURL(parsedURL)
}

func NewUpstreamOrDie(rawUrl string) Upstream {
	parsedURL, err := url.Parse(rawUrl)
	if err != nil {
		panic(err)
	}
	upstream, err := FromURL(parsedURL)
	if err != nil {
		panic(err)
	}
	return upstream
}

func FromURL(parsedURL *url.URL) (Upstream, error) {
	intPort, err := strconv.Atoi(parsedURL.Port())
	return &CustomUpstream{
		Scheme: parsedURL.Scheme,
		Host:   parsedURL.Hostname(),
		Port:   intPort,
		Path:   parsedURL.Path,
	}, err
}

func (u *CustomUpstream) GetScheme() string {
	return u.Scheme
}

func (u *CustomUpstream) GetHostname() string {
	return u.Host
}

func (u *CustomUpstream) GetPort() int {
	return u.Port
}

func (u *CustomUpstream) GetPath() string {
	return u.Path
}
