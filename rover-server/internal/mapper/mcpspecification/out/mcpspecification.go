// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"context"

	"github.com/pkg/errors"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func MapResponse(ctx context.Context, in *roverv1.McpSpecification, inFile map[string]any, stores *store.Stores) (res api.McpSpecificationResponse, err error) {
	if in == nil {
		return res, errors.New("input mcp specification is nil")
	}

	res = api.McpSpecificationResponse{
		Category: in.Spec.Category,
		Id:       mapper.MakeResourceId(in),
		Name:     in.Name,
	}

	if inFile != nil {
		res.Specification = inFile
	} else {
		res.Specification = map[string]any{}
	}

	res.Status, err = status.MapMcpSpecificationResponse(ctx, in, stores)
	return res, err
}
