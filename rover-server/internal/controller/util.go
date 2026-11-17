// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func EnsureLabelsOrDie(ctx context.Context, obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	defer obj.SetLabels(labels)

	bCtx, ok := security.FromContext(ctx)
	if !ok {
		panic("security context not found")
	}

	labels[config.EnvironmentLabelKey] = labelutil.NormalizeLabelValue(bCtx.Environment)
	labels[config.BuildLabelKey("team")] = labelutil.NormalizeLabelValue(bCtx.Team)
	labels[config.BuildLabelKey("group")] = labelutil.NormalizeLabelValue(bCtx.Group)
}

// resolveFileId returns the file ID already stored on the resource, or a new UUIDv7 if none exists.
func resolveFileId(existingFileId string) (string, error) {
	if existingFileId != "" {
		return existingFileId, nil
	}
	newId, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generating file id: %w", err)
	}
	return newId.String(), nil
}

// specToYAML converts the JSON specification of a request into the stored YAML,
// keeping the key order of the request.
func specToYAML(spec json.RawMessage) ([]byte, error) {
	var doc any
	if err := yaml.UnmarshalWithOptions(spec, &doc, yaml.UseOrderedMap()); err != nil {
		return nil, errors.Wrap(err, "decoding specification")
	}
	switch doc.(type) {
	case yaml.MapSlice:
		return yaml.MarshalWithOptions(doc, yaml.IndentSequence(true))
	case nil:
		return nil, errors.New("spec is empty")
	default:
		return nil, errors.New("specification must be an object")
	}
}

// specToJSON converts a stored YAML specification into compact JSON, keeping
// the key order of the file.
func specToJSON(data []byte) (json.RawMessage, error) {
	out, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, errors.Wrap(err, "decoding specification")
	}
	// The response encoder does not validate raw JSON.
	var buf bytes.Buffer
	if err := json.Compact(&buf, out); err != nil {
		return nil, errors.Wrap(err, "converting specification to JSON")
	}
	return buf.Bytes(), nil
}
