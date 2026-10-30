// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
)

const entityListPageSize = 1000

// kind adapts one Kong entity type to discovery and to the located entity: get by ID or
// name, list by tags, read ID, name and tags, and build the plain entity for an ID or name.
type kind[Req, Resp any] struct {
	name       string
	collection string
	plain      func(api KongAdminApi, idOrName string) entity[Req, Resp]
	listPage   func(ctx context.Context, api KongAdminApi, offset, tags *string) (int, []byte, error)
	fields     func(*Resp) (id, name *string, tags *[]string)
	reqTags    func(*Req) **[]string
}

func (k kind[Req, Resp]) get(ctx context.Context, api KongAdminApi, idOrName string) (*Resp, error) {
	current, found, err := k.plain(api, idOrName).Get(ctx)
	if err != nil || !found {
		return nil, err
	}
	return current, nil
}

func (k kind[Req, Resp]) list(ctx context.Context, api KongAdminApi, tags []string) ([]Resp, error) {
	return listAll[Resp](ctx, k.collection, func(offset *string) (int, []byte, error) {
		return k.listPage(ctx, api, offset, encodeTags(tags))
	})
}

func (k kind[Req, Resp]) id(r *Resp) string {
	id, _, _ := k.fields(r)
	return valueOrZero(id)
}

func (k kind[Req, Resp]) entName(r *Resp) string {
	_, name, _ := k.fields(r)
	return valueOrZero(name)
}

func (k kind[Req, Resp]) tags(r *Resp) []string {
	_, _, tags := k.fields(r)
	return valueOrZero(tags)
}

// pageResult unpacks a generated list response.
func pageResult[R interface{ StatusCode() int }](response R, body func(R) []byte, err error) (int, []byte, error) {
	if err != nil {
		return 0, nil, err
	}
	return response.StatusCode(), body(response), nil
}

var routeKind = kind[kong.CreateRouteJSONRequestBody, kong.Route]{
	name: "route", collection: "routes",
	plain: func(api KongAdminApi, n string) entity[kong.CreateRouteJSONRequestBody, kong.Route] {
		return routeEntity{api, n}
	},
	listPage: listRoutePage,
	fields:   func(r *kong.Route) (*string, *string, *[]string) { return r.Id, r.Name, r.Tags },
	reqTags:  func(r *kong.CreateRouteJSONRequestBody) **[]string { return &r.Tags },
}

var serviceKind = kind[kong.CreateServiceJSONRequestBody, kong.Service]{
	name: "service", collection: "services",
	plain: func(api KongAdminApi, n string) entity[kong.CreateServiceJSONRequestBody, kong.Service] {
		return serviceEntity{api, n}
	},
	listPage: listServicePage,
	fields:   func(s *kong.Service) (*string, *string, *[]string) { return s.Id, s.Name, s.Tags },
	reqTags:  func(s *kong.CreateServiceJSONRequestBody) **[]string { return &s.Tags },
}

var upstreamKind = kind[kong.CreateUpstreamJSONRequestBody, kong.Upstream]{
	name: "upstream", collection: "upstreams",
	plain: func(api KongAdminApi, n string) entity[kong.CreateUpstreamJSONRequestBody, kong.Upstream] {
		return upstreamEntity{api, n}
	},
	listPage: listUpstreamPage,
	fields:   func(u *kong.Upstream) (*string, *string, *[]string) { return u.Id, u.Name, u.Tags },
	reqTags:  func(u *kong.CreateUpstreamJSONRequestBody) **[]string { return &u.Tags },
}

func listRoutePage(ctx context.Context, api KongAdminApi, offset, tags *string) (int, []byte, error) {
	r, err := api.ListRouteWithResponse(ctx, &kong.ListRouteParams{Size: ptrOf(entityListPageSize), Offset: offset, Tags: tags})
	return pageResult(r, func(r *kong.ListRouteResponse) []byte { return r.Body }, err)
}

func listServicePage(ctx context.Context, api KongAdminApi, offset, tags *string) (int, []byte, error) {
	r, err := api.ListServiceWithResponse(ctx, &kong.ListServiceParams{Size: ptrOf(entityListPageSize), Offset: offset, Tags: tags})
	return pageResult(r, func(r *kong.ListServiceResponse) []byte { return r.Body }, err)
}

func listUpstreamPage(ctx context.Context, api KongAdminApi, offset, tags *string) (int, []byte, error) {
	r, err := api.ListUpstreamWithResponse(ctx, &kong.ListUpstreamParams{Size: ptrOf(entityListPageSize), Offset: offset, Tags: tags})
	return pageResult(r, func(r *kong.ListUpstreamResponse) []byte { return r.Body }, err)
}

// listAll reads every page of a Kong list endpoint. The body is decoded directly, because
// the generated response types of the list endpoints are inconsistent.
func listAll[T any](ctx context.Context, what string, fetch func(offset *string) (int, []byte, error)) ([]T, error) {
	var items []T
	var offset *string
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		status, body, err := fetch(offset)
		if err != nil {
			return nil, fmt.Errorf("failed to list %s: %w", what, HandleClientError(err))
		}
		if err := CheckStatusCode(statusCode(status), http.StatusOK); err != nil {
			return nil, fmt.Errorf("failed to list %s (%d): %s: %w", what, status, summarizeBody(body), err)
		}
		var page struct {
			Data   []T     `json:"data"`
			Offset *string `json:"offset"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to decode %s list: %w", what, err)
		}
		if page.Data == nil {
			return nil, fmt.Errorf("failed to list %s: %w", what, errors.New("response data is missing"))
		}
		items = append(items, page.Data...)
		if page.Offset == nil || *page.Offset == "" {
			return items, nil
		}
		if seen[*page.Offset] {
			return nil, fmt.Errorf("failed to list %s: %w", what, errors.New("offset repeats"))
		}
		seen[*page.Offset] = true
		offset = page.Offset
	}
}
