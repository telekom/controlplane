// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// ApiRoadmapHandler is a specialized handler for ApiRoadmap resources
type ApiRoadmapHandler struct {
	*common.BaseHandler
}

func NewApiRoadmapHandlerInstance() *ApiRoadmapHandler {
	handler := &ApiRoadmapHandler{
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "ApiRoadmap", "apiroadmaps", 10).WithValidation(common.ValidateObjectName),
	}

	handler.AddHook(common.PreRequestHook, PatchApiRoadmapRequest)
	return handler
}

func PatchApiRoadmapRequest(ctx context.Context, obj types.Object) error {
	if obj == nil {
		return nil
	}
	spec, ok := obj.GetContent()["spec"]
	if !ok {
		return errors.New("invalid ApiRoadmap. Missing 'spec'.")
	}
	specMap, ok := spec.(map[string]any)
	if !ok {
		return errors.New("invalid ApiRoadmap. 'spec' should be an object.")
	}

	// We only care about the spec part when sending it to the API, so we replace the content with the spec map directly
	obj.SetContent(specMap)
	return nil
}
