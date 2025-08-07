// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"

	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// ApiSpecHandler is a specialized handler for ApiSpecification resources
type ApiSpecHandler struct {
	*common.BaseHandler
}

func NewApiSpecHandlerInstance() *ApiSpecHandler {
	handler := &ApiSpecHandler{
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "ApiSpecification", "apispecifications", 10),
	}

	handler.AddHook(common.PreRequestHook, PatchApiSpecificationRequest)
	return handler
}

func PatchApiSpecificationRequest(ctx context.Context, obj types.Object) error {
	content := map[string]any{
		"specification": obj.GetContent(),
	}
	obj.SetContent(content)
	return nil
}
