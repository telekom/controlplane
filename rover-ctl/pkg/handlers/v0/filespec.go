// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

type FileSpecHandler struct {
	*common.BaseHandler
}

func NewFileSpecHandlerInstance() *FileSpecHandler {
	handler := &FileSpecHandler{
		BaseHandler: common.NewBaseHandler("rover.cp.ei.telekom.de/v1", "FileSpecification", "filespecifications", 10).WithValidation(common.ValidateObjectName),
	}
	handler.AddHook(common.PreRequestHook, PatchFileSpecificationRequest)
	return handler
}

func PatchFileSpecificationRequest(_ context.Context, obj types.Object) error {
	if obj == nil {
		return errors.New("FileSpecification object is nil")
	}
	spec, ok := obj.GetContent()["spec"]
	if !ok {
		return errors.New("invalid FileSpecification: missing 'spec'")
	}
	specMap, ok := spec.(map[string]any)
	if !ok {
		return errors.New("invalid FileSpecification: 'spec' should be an object")
	}
	obj.SetContent(specMap)
	return nil
}
