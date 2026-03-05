// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// EventSpecHandler is a specialized handler for EventSpecification resources
type EventSpecHandler struct {
	*common.BaseHandler
}

func NewEventSpecHandlerInstance() *EventSpecHandler {
	handler := &EventSpecHandler{
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "EventSpecification", "eventspecifications", 10).WithValidation(common.ValidateObjectName),
	}

	handler.AddHook(common.PreRequestHook, PatchEventSpecificationRequest)
	return handler
}

func PatchEventSpecificationRequest(ctx context.Context, obj types.Object) error {
	spec, ok := obj.GetContent()["spec"]
	if !ok {
		return errors.New("invalid EventSpecification. Missing 'spec'.")
	}
	specMap, ok := spec.(map[string]any)
	if !ok {
		return errors.New("invalid EventSpecification. 'spec' should be an object.")
	}

	// Determine the base directory of the resource file for resolving relative file:// refs
	var baseDir string
	if filename, ok := obj.GetProperty("filename").(string); ok && filename != "" {
		baseDir = filepath.Dir(filename)
	}

	jsonSchema, ok := specMap["specification"]
	if ok {
		switch v := jsonSchema.(type) {
		case string:
			var schemaMap map[string]any
			err := json.Unmarshal([]byte(v), &schemaMap)
			if err != nil {
				return errors.Wrap(err, "failed to parse JSON schema")
			}

			specMap["specification"], err = resolveJsonSchemaReference(schemaMap, baseDir)
			if err != nil {
				return errors.Wrap(err, "failed to resolve JSON schema reference")
			}
		case map[string]any:
			resolvedSchema, err := resolveJsonSchemaReference(v, baseDir)
			if err != nil {
				return errors.Wrap(err, "failed to resolve JSON schema reference")
			}
			specMap["specification"] = resolvedSchema
		default:
			return errors.New("invalid EventSpecification. 'specification' should be a JSON string or an object.")
		}
	}

	// No need to call obj.SetContent() here — specMap is a reference to the map
	// inside the content, so modifications to specMap["specification"] are already
	// reflected in the object's content.
	return nil
}

func resolveJsonSchemaReference(jsonSchema map[string]any, baseDir string) (map[string]any, error) {
	if ref, ok := jsonSchema["$ref"]; ok {
		refStr, ok := ref.(string)
		if !ok {
			return nil, errors.New("invalid $ref value in JSON schema")
		}
		if refPath, ok := strings.CutPrefix(refStr, "file://"); ok {
			// Resolve relative paths against the resource file's directory
			if baseDir != "" && !filepath.IsAbs(refPath) {
				refPath = filepath.Join(baseDir, refPath)
			}
			stat, err := os.Stat(refPath)
			if err != nil {
				return nil, errors.Wrap(err, "failed to access JSON schema file")
			}
			if stat.IsDir() {
				return nil, errors.New("JSON schema reference points to a directory, expected a file")
			}
			data, err := os.ReadFile(refPath)
			if err != nil {
				return nil, errors.Wrap(err, "failed to read JSON schema file")
			}
			var schemaMap map[string]any
			if err := json.Unmarshal(data, &schemaMap); err != nil {
				return nil, errors.Wrap(err, "failed to parse JSON schema from file")
			}
			return schemaMap, nil
		}

	}
	return jsonSchema, nil

}
