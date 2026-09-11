// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ server.ResourcesController = &ResourcesControllerImpl{}

const (
	resourceCursorVersion = 1
)

type resourceCursor struct {
	Version int    `json:"v"`
	Kind    int    `json:"k"`
	Cursor  string `json:"c"`
}

func encodeResourceCursor(cursor resourceCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encoding resource cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeResourceCursor(value string) (resourceCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return resourceCursor{}, problems.BadRequest("invalid cursor")
	}

	var cursor resourceCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != resourceCursorVersion || cursor.Kind < 0 || cursor.Kind >= len(resourceKinds) {
		return resourceCursor{}, problems.BadRequest("invalid cursor")
	}
	return cursor, nil
}

type ResourcesControllerImpl struct {
	stores *s.Stores
}

func NewResourcesController(stores *s.Stores) *ResourcesControllerImpl {
	return &ResourcesControllerImpl{stores: stores}
}

// resourceKind groups the metadata needed to build ResourceRef entries for a store.
type resourceKind struct {
	apiVersion string
	kind       string
	pathPrefix string
}

var resourceKinds = []resourceKind{
	{apiVersion: roverv1.GroupVersion.String(), kind: "Rover", pathPrefix: "/rovers"},
	{apiVersion: roverv1.GroupVersion.String(), kind: "ApiSpecification", pathPrefix: "/apispecifications"},
	{apiVersion: roverv1.GroupVersion.String(), kind: "EventSpecification", pathPrefix: "/eventspecifications"},
	{apiVersion: roverv1.GroupVersion.String(), kind: "Roadmap", pathPrefix: "/apiroadmaps"},
	{apiVersion: roverv1.GroupVersion.String(), kind: "ApiChangelog", pathPrefix: "/apichangelogs"},
	{apiVersion: roverv1.GroupVersion.String(), kind: "McpSpecification", pathPrefix: "/mcpspecifications"},
}

func (r *ResourcesControllerImpl) GetAll(ctx context.Context, params api.GetAllResourcesParams) (*api.ResourceListResponse, error) {
	_, _, effectivePrefix, err := resolveResourceTeam(ctx, params)
	if err != nil {
		return nil, err
	}

	cursor := resourceCursor{Version: resourceCursorVersion}
	if params.Cursor != "" {
		cursor, err = decodeResourceCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
	}

	limit := int(params.Limit)
	if limit == 0 {
		limit = store.DefaultPageSize
	}
	var items []api.ResourceRef
	next := ""
	for cursor.Kind < len(resourceKinds) && len(items) < limit {
		remaining := limit - len(items)
		page, _, storeNext, err := r.collectResourcePage(ctx, effectivePrefix, cursor, remaining)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if storeNext != "" {
			if storeNext == cursor.Cursor {
				return nil, fmt.Errorf("%s cursor did not advance", resourceKinds[cursor.Kind].kind)
			}
			next, err = encodeResourceCursor(resourceCursor{Version: resourceCursorVersion, Kind: cursor.Kind, Cursor: storeNext})
			if err != nil {
				return nil, err
			}
			break
		}

		cursor.Kind++
		cursor.Cursor = ""
	}

	if len(items) == limit && next == "" {
		for cursor.Kind < len(resourceKinds) {
			page, self, _, err := r.collectResourcePage(ctx, effectivePrefix, cursor, 1)
			if err != nil {
				return nil, err
			}
			if len(page) > 0 {
				next, err = encodeResourceCursor(resourceCursor{Version: resourceCursorVersion, Kind: cursor.Kind, Cursor: self})
				if err != nil {
					return nil, err
				}
				break
			}
			cursor.Kind++
		}
	}

	return &api.ResourceListResponse{
		UnderscoreLinks: api.Links{
			Self: params.Cursor,
			Next: next,
		},
		Items: items,
	}, nil
}

func resolveResourceTeam(ctx context.Context, params api.GetAllResourcesParams) (string, string, string, error) {
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return "", "", "", problems.InternalServerError("Invalid Context", "Security context not found")
	}

	group, team := params.Group, params.Team
	if (group == "") != (team == "") {
		return "", "", "", problems.BadRequest("both 'group' and 'team' query parameters must be provided together")
	}

	switch bCtx.ClientType {
	case security.ClientTypeTeam:
		if group == "" {
			group, team = bCtx.Group, bCtx.Team
		} else if group != bCtx.Group || team != bCtx.Team {
			return "", "", "", problems.Forbidden("access denied", "requested group/team is outside your access scope")
		}
	case security.ClientTypeGroup:
		if group == "" {
			return "", "", "", problems.BadRequest("both 'group' and 'team' query parameters must be provided")
		}
		if group != bCtx.Group {
			return "", "", "", problems.Forbidden("access denied", "requested group/team is outside your access scope")
		}
	case security.ClientTypeAdmin:
		if group == "" {
			return "", "", "", problems.BadRequest("both 'group' and 'team' query parameters must be provided")
		}
	default:
		return "", "", "", problems.Forbidden("access denied", "unsupported client type")
	}

	return group, team, fmt.Sprintf("%s--%s--%s/", bCtx.Environment, group, team), nil
}

func collectResourcePage[T store.Object](
	ctx context.Context,
	prefix string,
	objStore store.ObjectStore[T],
	rk resourceKind,
	limit int,
	cursor string,
) (items []api.ResourceRef, self, next string, err error) {
	listOpts := store.NewListOpts()
	store.EnforcePrefix(prefix, &listOpts)
	listOpts.Limit = limit
	listOpts.Cursor = cursor

	objList, err := objStore.List(ctx, listOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("listing %s: %w", rk.kind, err)
	}

	for _, item := range objList.Items {
		items = append(items, api.ResourceRef{
			ApiVersion: rk.apiVersion,
			Kind:       rk.kind,
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Path:       fmt.Sprintf("%s/%s", rk.pathPrefix, mapper.MakeResourceId(item)),
		})
	}

	return items, objList.Links.Self, objList.Links.Next, nil
}

func (r *ResourcesControllerImpl) collectResourcePage(
	ctx context.Context,
	prefix string,
	cursor resourceCursor,
	limit int,
) (items []api.ResourceRef, self, next string, err error) {
	rk := resourceKinds[cursor.Kind]
	switch cursor.Kind {
	case 0:
		return collectResourcePage(ctx, prefix, r.stores.RoverStore, rk, limit, cursor.Cursor)
	case 1:
		return collectResourcePage(ctx, prefix, r.stores.APISpecificationStore, rk, limit, cursor.Cursor)
	case 2:
		return collectResourcePage(ctx, prefix, r.stores.EventSpecificationStore, rk, limit, cursor.Cursor)
	case 3:
		return collectResourcePage(ctx, prefix, r.stores.RoadmapStore, rk, limit, cursor.Cursor)
	case 4:
		return collectResourcePage(ctx, prefix, r.stores.ApiChangelogStore, rk, limit, cursor.Cursor)
	case 5:
		return collectResourcePage(ctx, prefix, r.stores.McpSpecificationStore, rk, limit, cursor.Cursor)
	default:
		return nil, "", "", fmt.Errorf("invalid resource kind %d", cursor.Kind)
	}
}
