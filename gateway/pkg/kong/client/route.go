// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"
)

// realmOf returns the realm view of a route if it has a name prefix.
func realmOf(route CustomRoute) (RealmRoute, bool) {
	realm, ok := route.(RealmRoute)
	if !ok || realm.GetNamePrefix() == "" {
		return nil, false
	}
	return realm, true
}

// routeTags are the tags of the route and service of a route.
func routeTags(ctx context.Context, route CustomRoute) []string {
	tags := []string{
		BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
		BuildTag("route", route.GetName()),
	}
	if realm, ok := realmOf(route); ok {
		tags = append(tags, BuildTag("realm", realm.GetNamePrefix()))
	}
	return tags
}

// discoveryFor builds the discovery of an entity kind for a realm route.
func discoveryFor[Req, Resp any](ctx context.Context, c *kongClient, k kind[Req, Resp], realm RealmRoute, route CustomRoute, storedID string, policy discoveryPolicy) *discovery[Req, Resp] {
	return &discovery[Req, Resp]{
		api:      c.client,
		kind:     k,
		storedID: storedID,
		names:    c.names.Names(legacyRoute(realm, route)),
		env:      contextutil.EnvFromContextOrDie(ctx),
		route:    route.GetName(),
		legacy:   true,
		policy:   policy,
	}
}

func legacyRoute(realm RealmRoute, route CustomRoute) legacy.Route {
	return legacy.Route{Name: route.GetName(), NamePrefix: realm.GetNamePrefix(), Labels: realm.GetLabels()}
}

// routeEntities are the route and service entities of one route.
type routeEntities struct {
	name    string
	route   entity[kong.CreateRouteJSONRequestBody, kong.Route]
	service entity[kong.CreateServiceJSONRequestBody, kong.Service]
	located *locatedEntity[kong.CreateRouteJSONRequestBody, kong.Route]
	svc     *locatedEntity[kong.CreateServiceJSONRequestBody, kong.Service]
}

// entitiesFor returns plain entities named after the route, or for a realm route located
// entities after discovering the route and its service. Discovery writes nothing.
func (c *kongClient) entitiesFor(ctx context.Context, route CustomRoute, policy discoveryPolicy) (*routeEntities, error) {
	realm, ok := realmOf(route)
	if !ok {
		name := route.GetName()
		return &routeEntities{
			name:    name,
			route:   routeEntity{client: c.client, name: name},
			service: serviceEntity{client: c.client, name: name},
		}, nil
	}

	ids := realm.GetStoredIds()
	located := newLocatedEntity(discoveryFor(ctx, c, routeKind, realm, route, ids.RouteId, policy))
	current, err := located.locate(ctx)
	if err != nil {
		return nil, err
	}
	serviceDiscovery := discoveryFor(ctx, c, serviceKind, realm, route, ids.ServiceId, policy)
	ownRouteID := ""
	if current != nil {
		ownRouteID = valueOrZero(current.Id)
		if current.Service != nil {
			serviceDiscovery.parentID = valueOrZero(current.Service.Id)
		}
	}
	serviceDiscovery.sharedWith = c.otherRouteOf(ownRouteID)
	svc := newLocatedEntity(serviceDiscovery)
	if _, err := svc.locate(ctx); err != nil {
		return nil, err
	}
	return &routeEntities{
		name:    realm.GetKongName(),
		route:   located,
		service: svc,
		located: located,
		svc:     svc,
	}, nil
}

// otherRouteOf returns a lookup of a Kong route other than ownRouteID that uses a service.
func (c *kongClient) otherRouteOf(ownRouteID string) func(ctx context.Context, service *kong.Service) (string, error) {
	return func(ctx context.Context, service *kong.Service) (string, error) {
		routes, err := listAll[kong.Route](ctx, "service routes", func(offset *string) (int, []byte, error) {
			r, err := c.client.ListRoutesForServiceWithResponse(ctx, valueOrZero(service.Id), &kong.ListRoutesForServiceParams{Size: ptrOf(entityListPageSize), Offset: offset})
			return pageResult(r, func(r *kong.ListRoutesForServiceResponse) []byte { return r.Body }, err)
		})
		if err != nil {
			return "", err
		}
		for i := range routes {
			if id := valueOrZero(routes[i].Id); id != ownRouteID {
				return fmt.Sprintf("route %q (%s)", valueOrZero(routes[i].Name), id), nil
			}
		}
		return "", nil
	}
}

// CreateOrReplaceRoute discovers route, service and, for a CircuitBreakerUpstream or an
// UpstreamRemoval, the upstream before it writes anything. Then it writes the upstream, the
// service and the route, and finally deletes a removed upstream.
func (c *kongClient) CreateOrReplaceRoute(ctx context.Context, route CustomRoute, upstream Upstream) error {
	if upstream == nil {
		return fmt.Errorf("upstream is required")
	}

	entities, err := c.entitiesFor(ctx, route, reconcilePolicy{})
	if err != nil {
		return err
	}
	managed, removal, err := c.prepareUpstreamOf(ctx, route, upstream)
	if err != nil {
		return err
	}
	if managed != nil {
		err = c.writeUpstream(ctx, route, managed)
		if err != nil {
			return err
		}
	}

	routeName := entities.name
	upstreamPath := upstream.GetPath()
	serviceName := routeName
	serviceHostname := upstream.GetHostname()
	tags := routeTags(ctx, route)

	serviceBody := kong.CreateServiceJSONRequestBody{
		Enabled:  true,
		Name:     &serviceName,
		Host:     serviceHostname,
		Path:     &upstreamPath,
		Protocol: kong.CreateServiceRequestProtocol(upstream.GetScheme()),
		Port:     upstream.GetPort(),
		Tags:     normalizeSet(&tags),
	}

	service, _, err := reconcile(ctx, entities.service, serviceBody)
	if err != nil {
		return err
	}
	if service.Id == nil {
		return fmt.Errorf("service response ID is missing")
	}
	route.SetServiceId(*service.Id)

	routeTags := slices.Concat(tags, route.GetAdditionalTags())

	routeBody := kong.CreateRouteJSONRequestBody{
		Name:                    &routeName,
		Protocols:               valueOrZero(normalizeSet(&[]string{"http", "https"})),
		Paths:                   toPtrOrNil(route.GetPaths()),
		Hosts:                   normalizeSet(toPtrOrNil(route.GetHostnames())),
		Service:                 &kong.CreateRouteRequestService{Id: service.Id},
		RequestBuffering:        route.GetRequestBuffering(),
		ResponseBuffering:       route.GetResponseBuffering(),
		PreserveHost:            route.IsPassThrough(),
		HttpsRedirectStatusCode: 426,
		Tags:                    normalizeSet(&routeTags),
	}
	if route.IsHealthProbe() {
		routeBody.Methods = &[]string{http.MethodHead}
	}

	kongRoute, _, err := reconcile(ctx, entities.route, routeBody)
	if err != nil {
		return err
	}
	if kongRoute.Id == nil {
		return fmt.Errorf("route response ID is missing")
	}
	route.SetRouteId(*kongRoute.Id)

	if entities.located != nil {
		if legacyName := entities.located.legacyName(); legacyName != "" {
			logr.FromContextOrDiscard(ctx).Info("Adopted legacy kong route", "legacyName", legacyName, "kongName", routeName, "routeId", *kongRoute.Id)
			if recorder, ok := route.(LegacyNameRecorder); ok {
				recorder.SetLegacyName(legacyName)
			}
		}
	}

	if removal != nil {
		return c.removeUpstream(ctx, route, *removal)
	}
	return nil
}

// prepareUpstreamOf locates the Kong upstream a CircuitBreakerUpstream writes or an
// UpstreamRemoval deletes. It writes nothing.
func (c *kongClient) prepareUpstreamOf(ctx context.Context, route CustomRoute, upstream Upstream) (*preparedUpstream, *string, error) {
	switch u := upstream.(type) {
	case *CircuitBreakerUpstream:
		prepared, err := c.prepareUpstream(ctx, route, u.Upstream, u.Target)
		return prepared, nil, err
	case *UpstreamRemoval:
		ref, err := c.upstreamRefForDelete(ctx, route)
		return nil, &ref, err
	default:
		return nil, nil, nil
	}
}

func (c *kongClient) DeleteRoute(ctx context.Context, route CustomRoute) error {
	if _, ok := realmOf(route); ok {
		return c.deleteRealmRoute(ctx, route)
	}

	routeName := route.GetName()
	if err := c.deleteRouteByIDOrName(ctx, routeName); err != nil {
		return err
	}
	if err := c.deleteServiceByIDOrName(ctx, routeName); err != nil {
		return err
	}
	return c.DeleteUpstream(ctx, route)
}

// deleteRealmRoute discovers the route, service and upstream without creating anything and
// deletes them by ID. Discovery skips entities that are not ours or are duplicates, so at
// most one entity per kind is deleted and a delete never blocks on them.
func (c *kongClient) deleteRealmRoute(ctx context.Context, route CustomRoute) error {
	entities, err := c.entitiesFor(ctx, route, deletePolicy{})
	if err != nil {
		return err
	}
	upstreamRef, err := c.upstreamRefForDelete(ctx, route)
	if err != nil {
		return err
	}

	log := logr.FromContextOrDiscard(ctx)
	if id := entities.located.foundID(); id != "" {
		log.V(1).Info("Deleting kong route", "routeId", id)
		if err := c.deleteRouteByIDOrName(ctx, id); err != nil {
			return err
		}
	}
	if id := entities.svc.foundID(); id != "" {
		log.V(1).Info("Deleting kong service", "serviceId", id)
		if err := c.deleteServiceByIDOrName(ctx, id); err != nil {
			return err
		}
	}
	return c.deleteUpstreamAndTargets(ctx, route, upstreamRef)
}

func (c *kongClient) deleteRouteByIDOrName(ctx context.Context, idOrName string) error {
	routeResponse, err := c.client.DeleteRouteWithResponse(ctx, idOrName)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", HandleClientError(err))
	}
	if err := CheckStatusCode(routeResponse, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
		return fmt.Errorf("failed to delete route (%d): %s: %w", routeResponse.StatusCode(), summarizeBody(routeResponse.Body), err)
	}
	return nil
}

func (c *kongClient) deleteServiceByIDOrName(ctx context.Context, idOrName string) error {
	serviceResponse, err := c.client.DeleteServiceWithResponse(ctx, idOrName)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", HandleClientError(err))
	}
	if err := CheckStatusCode(serviceResponse, http.StatusOK, http.StatusNoContent, http.StatusNotFound); err != nil {
		return fmt.Errorf("failed to delete service (%d): %s: %w", serviceResponse.StatusCode(), summarizeBody(serviceResponse.Body), err)
	}
	return nil
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
		Methods:                 normalizeSet(current.Methods),
		Service:                 service,
		RequestBuffering:        valueOrZero(current.RequestBuffering),
		ResponseBuffering:       valueOrZero(current.ResponseBuffering),
		PreserveHost:            valueOrZero(current.PreserveHost),
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
