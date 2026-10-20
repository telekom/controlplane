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
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
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
	if obj == nil {
		return nil
	}
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

	// We only care about the spec part when sending it to the API, so we can replace the content with the spec map directly
	obj.SetContent(specMap)
	return nil
}

func resolveJsonSchemaReference(jsonSchema map[string]any, baseDir string) (map[string]any, error) {
	if ref, ok := jsonSchema["$ref"]; ok {
		refStr, ok := ref.(string)
		if !ok {
			return nil, errors.New("invalid $ref value in JSON schema")
		}
		if refPath, ok := strings.CutPrefix(refStr, "file://"); ok {
			return loadJsonSchemaFile(refPath, baseDir)
		}
	}
	return jsonSchema, nil

}

// loadJsonSchemaFile loads a file:// reference. Relative paths are resolved against
// baseDir first. Only if that file does not exist, the resource-path location (resource.path,
// or the current working directory if unset) is tried.
func loadJsonSchemaFile(refPath, baseDir string) (map[string]any, error) {
	if filepath.IsAbs(refPath) {
		return readJsonSchemaFile(refPath)
	}

	primary := refPath
	if baseDir != "" {
		primary = filepath.Join(baseDir, refPath)
	}
	schema, err := readJsonSchemaFile(primary)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return schema, errors.Wrapf(err, "failed to load JSON schema file from primary location %q", primary)
	}

	fallback := refPath
	if resourcePath := config.ResourcePath(); resourcePath != "" {
		fallback = filepath.Join(resourcePath, refPath)
	}
	if filepath.Clean(fallback) == filepath.Clean(primary) {
		return nil, err
	}
	schema, fallbackErr := readJsonSchemaFile(fallback)
	if fallbackErr == nil {
		return schema, nil
	}
	if errors.Is(fallbackErr, os.ErrNotExist) {
		return nil, errors.Wrapf(err, "JSON schema file %q not found (tried %q and fallback location %q)", refPath, primary, fallback)
	}
	return nil, errors.Wrapf(fallbackErr, "failed to load JSON schema file from fallback location %q", fallback)
}

func readJsonSchemaFile(path string) (map[string]any, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to access JSON schema file")
	}
	if stat.IsDir() {
		return nil, errors.New("JSON schema reference points to a directory, expected a file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read JSON schema file")
	}
	var schemaMap map[string]any
	if err := json.Unmarshal(data, &schemaMap); err != nil {
		return nil, errors.Wrap(err, "failed to parse JSON schema from file")
	}
	return schemaMap, nil
}
