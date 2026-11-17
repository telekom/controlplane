// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// ApiSpecHandler is a specialized handler for ApiSpecification resources
type ApiSpecHandler struct {
	*common.BaseHandler
}

func NewApiSpecHandlerInstance() *ApiSpecHandler {
	handler := &ApiSpecHandler{
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "ApiSpecification", "apispecifications", 10).WithValidation(common.ValidateObjectName),
	}

	handler.AddHook(common.PreRequestHook, PatchApiSpecificationRequest)
	return handler
}

// PatchApiSpecificationRequest sends the specification in the key order of its source file.
func PatchApiSpecificationRequest(ctx context.Context, obj types.Object) error {
	if obj == nil {
		return nil
	}
	var specification any = obj.GetContent()
	switch ordered := obj.GetProperty(types.PropertyOrderedContent).(type) {
	case json.RawMessage:
		specification = ordered
	case yaml.MapSlice:
		raw, err := yaml.MarshalWithOptions(ordered, yaml.JSON())
		if err != nil {
			return fmt.Errorf("encoding API specification: %w", err)
		}
		specification = json.RawMessage(raw)
	}
	content := map[string]any{
		"specification": specification,
	}
	obj.SetContent(content)
	return nil
}
