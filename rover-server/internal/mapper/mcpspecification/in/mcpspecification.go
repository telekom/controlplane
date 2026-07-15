// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"context"
	"strings"

	filesapi "github.com/telekom/controlplane/file-manager/api"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	config "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// ParseSpecification parses MCP specification YAML and extracts metadata.
func ParseSpecification(_ context.Context, specYAML string) (*roverv1.McpSpecification, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(specYAML), &raw); err != nil {
		return nil, problems.BadRequest("failed to parse MCP specification YAML: " + err.Error())
	}

	basePath, _ := raw["basePath"].(string)
	if basePath == "" {
		return nil, problems.BadRequest("basePath is required in MCP specification")
	}

	name := roverv1.MakeMcpSpecificationName(basePath)
	version := "1.0.0"
	description := ""
	category := "other"

	if info, ok := raw["info"].(map[string]any); ok {
		if v, ok := info["version"].(string); ok && v != "" {
			version = v
		}
		if title, ok := info["title"].(string); ok && title != "" {
			name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(title), " ", "-"))
		}
		if d, ok := info["description"].(string); ok {
			description = d
		}
	}

	if cat, ok := raw["x-api-category"].(string); ok && cat != "" {
		category = cat
	}

	var scopes []string
	if scopeList, ok := raw["scopes"].([]any); ok {
		for _, scope := range scopeList {
			if str, ok := scope.(string); ok {
				scopes = append(scopes, str)
			}
		}
	}

	return &roverv1.McpSpecification{
		ObjectMeta: metav1.ObjectMeta{
			Name: roverv1.MakeMcpSpecificationName(basePath),
		},
		Spec: roverv1.McpSpecificationSpec{
			BasePath:     basePath,
			Name:         name,
			Version:      version,
			Description:  description,
			Category:     category,
			Oauth2Scopes: scopes,
		},
	}, nil
}

// MapRequest maps file manager response to McpSpecification CR fields.
func MapRequest(mcpSpec *roverv1.McpSpecification, fileAPIResp *filesapi.FileUploadResponse, id mapper.ResourceIdInfo) {
	mcpSpec.TypeMeta = metav1.TypeMeta{
		Kind:       "McpSpecification",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}
	mcpSpec.Spec.Hash = fileAPIResp.FileHash
	mcpSpec.Spec.Specification = fileAPIResp.FileId
	mcpSpec.Labels = map[string]string{
		config.EnvironmentLabelKey: id.Environment,
	}
	mcpSpec.Namespace = id.Environment + "--" + id.Namespace
}

// MapRequestWithoutFile maps McpSpecification fields when file-manager is disabled.
func MapRequestWithoutFile(mcpSpec *roverv1.McpSpecification, id mapper.ResourceIdInfo) {
	mcpSpec.TypeMeta = metav1.TypeMeta{
		Kind:       "McpSpecification",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}
	mcpSpec.Labels = map[string]string{
		config.EnvironmentLabelKey: id.Environment,
	}
	mcpSpec.Namespace = id.Environment + "--" + id.Namespace
}
