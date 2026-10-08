// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an McpServer CR to an McpServerData DTO and derives
// identity keys.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*agenticv1.McpServer, *McpServerData, McpServerKey] = (*Translator)(nil)

// ShouldSkip returns false — McpServer CRs are always syncable.
func (t *Translator) ShouldSkip(_ *agenticv1.McpServer) (bool, string) {
	return false, ""
}

// Translate converts an McpServer CR into an McpServerData DTO.
func (t *Translator) Translate(_ context.Context, obj *agenticv1.McpServer) (*McpServerData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	scopes := obj.Spec.Oauth2Scopes
	if scopes == nil {
		scopes = []string{}
	}

	return &McpServerData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		BasePath:      obj.Spec.BasePath,
		Version:       obj.Spec.Version,
		Name:          obj.Name,
		DisplayName:   obj.Spec.Name,
		Description:   obj.Spec.Description,
		Category:      obj.Spec.Category,
		Oauth2Scopes:  scopes,
		Specification: obj.Spec.Specification,
		Hash:          obj.Spec.Hash,
		Active:        obj.Status.Active,
		TeamName:      shared.TeamNameFromNamespace(obj.Namespace),
	}, nil
}

// KeyFromObject derives the composite identity key from a live McpServer CR.
func (t *Translator) KeyFromObject(obj *agenticv1.McpServer) McpServerKey {
	return McpServerKey{
		BasePath:  obj.Spec.BasePath,
		TeamName:  shared.TeamNameFromNamespace(obj.Namespace),
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, uses its Spec.BasePath, namespace and name.
// Otherwise, the base path is unknown: the key holds only the namespace and
// name from req, and Delete finds the entity by them.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *agenticv1.McpServer) (McpServerKey, error) {
	if lastKnown != nil {
		return McpServerKey{
			BasePath:  lastKnown.Spec.BasePath,
			TeamName:  shared.TeamNameFromNamespace(lastKnown.Namespace),
			Namespace: lastKnown.Namespace,
			Name:      lastKnown.Name,
		}, nil
	}
	return McpServerKey{
		TeamName:  shared.TeamNameFromNamespace(req.Namespace),
		Namespace: req.Namespace,
		Name:      req.Name,
	}, nil
}
