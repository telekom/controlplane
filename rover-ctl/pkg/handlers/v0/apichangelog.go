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

// ApiChangelogHandler is a specialized handler for ApiChangelog resources
type ApiChangelogHandler struct {
	*common.BaseHandler
}

func NewApiChangelogHandlerInstance() *ApiChangelogHandler {
	handler := &ApiChangelogHandler{
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "ApiChangelog", "apichangelogs", 10).WithValidation(common.ValidateObjectName),
	}

	handler.AddHook(common.PreRequestHook, PatchApiChangelogRequest)
	return handler
}

func PatchApiChangelogRequest(ctx context.Context, obj types.Object) error {
	if obj == nil {
		return nil
	}
	spec, ok := obj.GetContent()["spec"]
	if !ok {
		return errors.New("invalid ApiChangelog. Missing 'spec'.")
	}
	specMap, ok := spec.(map[string]any)
	if !ok {
		return errors.New("invalid ApiChangelog. 'spec' should be an object.")
	}

	// We only care about the spec part when sending it to the API, so we replace the content with the spec map directly
	obj.SetContent(specMap)
	return nil
}
