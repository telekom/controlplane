// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

func (c *kongClient) CreateOrReplaceRoute(ctx context.Context, route CustomRoute, upstream Upstream) error {
	if upstream == nil {
		return fmt.Errorf("upstream is required")
	}

	routeName := route.GetName()
	upstreamPath := upstream.GetPath()
	serviceName := routeName
	serviceHostname := upstream.GetHostname()
	tags := []string{
		BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
		BuildTag("route", routeName),
	}

	serviceBody := kong.CreateServiceJSONRequestBody{
		Enabled:  true,
		Name:     &serviceName,
		Host:     serviceHostname,
		Path:     &upstreamPath,
		Protocol: kong.CreateServiceRequestProtocol(upstream.GetScheme()),
		Port:     upstream.GetPort(),
		Tags:     normalizeSet(&tags),
	}

	service, _, err := reconcile(ctx, serviceEntity{client: c.client, name: routeName}, serviceBody)
	if err != nil {
		return err
	}
	if service.Id == nil {
		return fmt.Errorf("service response ID is missing")
	}
	route.SetServiceId(*service.Id)

	routeBody := kong.CreateRouteJSONRequestBody{
		Name:                    &routeName,
		Protocols:               valueOrZero(normalizeSet(&[]string{"http", "https"})),
		Paths:                   toPtrOrNil(route.GetPaths()),
		Hosts:                   normalizeSet(toPtrOrNil(route.GetHostnames())),
		Service:                 &kong.CreateRouteRequestService{Id: service.Id},
		RequestBuffering:        route.GetRequestBuffering(),
		ResponseBuffering:       route.GetResponseBuffering(),
		HttpsRedirectStatusCode: 426,
		Tags:                    normalizeSet(&tags),
	}

	kongRoute, _, err := reconcile(ctx, routeEntity{client: c.client, name: routeName}, routeBody)
	if err != nil {
		return err
	}
	if kongRoute.Id == nil {
		return fmt.Errorf("route response ID is missing")
	}
	route.SetRouteId(*kongRoute.Id)

	return nil
}

func (c *kongClient) DeleteRoute(ctx context.Context, route CustomRoute) error {
	routeName := route.GetName()
	routeResponse, err := c.client.DeleteRouteWithResponse(ctx, routeName)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", HandleClientError(err))
	}
	if err := CheckStatusCode(routeResponse, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
		return fmt.Errorf("failed to delete route (%d): %s: %w", routeResponse.StatusCode(), summarizeBody(routeResponse.Body), err)
	}

	serviceResponse, err := c.client.DeleteServiceWithResponse(ctx, routeName)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", HandleClientError(err))
	}
	if err := CheckStatusCode(serviceResponse, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
		return fmt.Errorf("failed to delete service (%d): %s: %w", serviceResponse.StatusCode(), summarizeBody(serviceResponse.Body), err)
	}

	return c.DeleteUpstream(ctx, route)
}

type serviceEntity struct {
	client KongAdminApi
	name   string
}

var _ entity[kong.CreateServiceJSONRequestBody, kong.Service] = serviceEntity{}

func (e serviceEntity) Name() string { return "service" }

func (e serviceEntity) Get(ctx context.Context) (*kong.Service, bool, error) {
	response, err := e.client.GetServiceWithResponse(ctx, e.name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get service: %w", HandleClientError(err))
	}
	return readOne("service", readResult[kong.Service]{response.StatusCode(), response.Body, response.JSON200})
}

func (e serviceEntity) Project(current *kong.Service) (kong.CreateServiceJSONRequestBody, error) {
	return kong.CreateServiceJSONRequestBody{
		Enabled:  valueOrZero(current.Enabled),
		Name:     current.Name,
		Host:     valueOrZero(current.Host),
		Path:     current.Path,
		Protocol: kong.CreateServiceRequestProtocol(valueOrZero(current.Protocol)),
		Port:     valueOrZero(current.Port),
		Tags:     normalizeSet(current.Tags),
	}, nil
}

func (e serviceEntity) Write(ctx context.Context, desired *kong.CreateServiceJSONRequestBody) (*kong.Service, error) {
	response, err := e.client.UpsertServiceWithResponse(ctx, e.name, *desired)
	if err != nil {
		return nil, fmt.Errorf("failed to write service: %w", HandleClientError(err))
	}
	return writeOne("service", readResult[kong.Service]{response.StatusCode(), response.Body, response.JSON200}, http.StatusOK)
}

// --- route -----------------------------------------------------------------

type routeEntity struct {
	client KongAdminApi
	name   string
}

var _ entity[kong.CreateRouteJSONRequestBody, kong.Route] = routeEntity{}

func (e routeEntity) Name() string { return "route" }

func (e routeEntity) Get(ctx context.Context) (*kong.Route, bool, error) {
	response, err := e.client.GetRouteWithResponse(ctx, e.name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get route: %w", HandleClientError(err))
	}
	return readOne("route", readResult[kong.Route]{response.StatusCode(), response.Body, response.JSON200})
}

func (e routeEntity) Project(current *kong.Route) (kong.CreateRouteJSONRequestBody, error) {
	var service *kong.CreateRouteRequestService
	if current.Service != nil {
		service = &kong.CreateRouteRequestService{Id: current.Service.Id}
	}
	return kong.CreateRouteJSONRequestBody{
		Name:                    current.Name,
		Protocols:               valueOrZero(normalizeSet(current.Protocols)),
		Paths:                   current.Paths,
		Hosts:                   normalizeSet(current.Hosts),
		Service:                 service,
		RequestBuffering:        valueOrZero(current.RequestBuffering),
		ResponseBuffering:       valueOrZero(current.ResponseBuffering),
		HttpsRedirectStatusCode: kong.CreateRouteRequestHttpsRedirectStatusCode(valueOrZero(current.HttpsRedirectStatusCode)),
		Tags:                    normalizeSet(current.Tags),
	}, nil
}

func (e routeEntity) Write(ctx context.Context, desired *kong.CreateRouteJSONRequestBody) (*kong.Route, error) {
	response, err := e.client.UpsertRouteWithResponse(ctx, e.name, *desired)
	if err != nil {
		return nil, fmt.Errorf("failed to write route: %w", HandleClientError(err))
	}
	return writeOne("route", readResult[kong.Route]{response.StatusCode(), response.Body, response.JSON200}, http.StatusOK)
}

// --- consumer --------------------------------------------------------------
