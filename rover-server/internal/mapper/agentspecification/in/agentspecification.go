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

// ParseSpecification parses agent specification YAML and extracts metadata.
func ParseSpecification(_ context.Context, specYAML string) (*roverv1.AgentSpecification, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(specYAML), &raw); err != nil {
		return nil, problems.BadRequest("failed to parse agent specification YAML: " + err.Error())
	}

	basePath, _ := raw["basePath"].(string)
	if basePath == "" {
		return nil, problems.BadRequest("basePath is required in agent specification")
	}

	name := roverv1.MakeAgentSpecificationName(basePath)
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

	return &roverv1.AgentSpecification{
		ObjectMeta: metav1.ObjectMeta{
			Name: roverv1.MakeAgentSpecificationName(basePath),
		},
		Spec: roverv1.AgentSpecificationSpec{
			BasePath:     basePath,
			Name:         name,
			Version:      version,
			Description:  description,
			Category:     category,
			Oauth2Scopes: scopes,
		},
	}, nil
}

// MapRequest maps file manager response to AgentSpecification CR fields.
func MapRequest(agentSpec *roverv1.AgentSpecification, fileAPIResp *filesapi.FileUploadResponse, id mapper.ResourceIdInfo) {
	agentSpec.TypeMeta = metav1.TypeMeta{
		Kind:       "AgentSpecification",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}
	agentSpec.Spec.Hash = fileAPIResp.FileHash
	agentSpec.Spec.Specification = fileAPIResp.FileId
	agentSpec.Labels = map[string]string{
		config.EnvironmentLabelKey: id.Environment,
	}
	agentSpec.Namespace = id.Environment + "--" + id.Namespace
}

// MapRequestWithoutFile maps AgentSpecification fields when file-manager is disabled.
func MapRequestWithoutFile(agentSpec *roverv1.AgentSpecification, id mapper.ResourceIdInfo) {
	agentSpec.TypeMeta = metav1.TypeMeta{
		Kind:       "AgentSpecification",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}
	agentSpec.Labels = map[string]string{
		config.EnvironmentLabelKey: id.Environment,
	}
	agentSpec.Namespace = id.Environment + "--" + id.Namespace
}
