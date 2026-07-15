// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
)

var _ server.ResourcesController = &ResourcesControllerImpl{}

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
}

func (r *ResourcesControllerImpl) GetAll(ctx context.Context, params api.GetAllResourcesParams) (*api.ResourceListResponse, error) {
	if params.Group == "" || params.Team == "" {
		return nil, problems.BadRequest("both 'group' and 'team' query parameters must be provided")
	}

	tokenPrefix := security.PrefixFromContext(ctx)

	effectivePrefix, err := buildTeamPrefix(tokenPrefix, params.Group, params.Team)
	if err != nil {
		return nil, err
	}

	var items []api.ResourceRef

	if err := collectFromStore(ctx, effectivePrefix, r.stores.RoverStore, resourceKinds[0], &items); err != nil {
		return nil, err
	}
	if err := collectFromStore(ctx, effectivePrefix, r.stores.APISpecificationStore, resourceKinds[1], &items); err != nil {
		return nil, err
	}
	if err := collectFromStore(ctx, effectivePrefix, r.stores.EventSpecificationStore, resourceKinds[2], &items); err != nil {
		return nil, err
	}
	if err := collectFromStore(ctx, effectivePrefix, r.stores.RoadmapStore, resourceKinds[3], &items); err != nil {
		return nil, err
	}
	if err := collectFromStore(ctx, effectivePrefix, r.stores.ApiChangelogStore, resourceKinds[4], &items); err != nil {
		return nil, err
	}

	selfLink := ""
	if params.Cursor != "" {
		selfLink = fmt.Sprintf("?cursor=%s", params.Cursor)
	}

	return &api.ResourceListResponse{
		UnderscoreLinks: api.Links{
			Self: selfLink,
			Next: "",
		},
		Items: items,
	}, nil
}

// buildTeamPrefix constructs the datastore prefix for the given group and team,
// and validates the caller's token has access to it.
// Token prefix examples: admin="env--", group="env--group--", team="env--group--team/"
// Effective prefix is always team-scoped: "env--group--team/"
func buildTeamPrefix(tokenPrefix string, group, team string) (string, error) {
	// Extract environment from the token prefix (everything before the first "--")
	env, _, ok := strings.Cut(tokenPrefix, "--")
	if !ok || env == "" {
		return "", problems.Forbidden("access denied", "unable to determine environment from token")
	}

	// Build the full team-level prefix
	effectivePrefix := fmt.Sprintf("%s--%s--%s/", env, group, team)

	// Verify the requested scope is within the caller's access scope
	if !strings.HasPrefix(effectivePrefix, tokenPrefix) {
		return "", problems.Forbidden("access denied", "requested group/team is outside your access scope")
	}

	return effectivePrefix, nil
}

func collectFromStore[T store.Object](
	ctx context.Context,
	prefix any,
	objStore store.ObjectStore[T],
	rk resourceKind,
	items *[]api.ResourceRef,
) error {
	listOpts := store.NewListOpts()
	store.EnforcePrefix(prefix, &listOpts)

	objList, err := objStore.List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("listing %s: %w", rk.kind, err)
	}

	for _, item := range objList.Items {
		ref := api.ResourceRef{
			ApiVersion: rk.apiVersion,
			Kind:       rk.kind,
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Path:       fmt.Sprintf("%s/%s", rk.pathPrefix, item.GetName()),
		}
		*items = append(*items, ref)
	}

	return nil
}
