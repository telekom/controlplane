// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard

import (
	"context"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an AgentCard CR to an AgentCardData DTO and derives
// identity keys.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*agenticv1.AgentCard, *AgentCardData, AgentCardKey] = (*Translator)(nil)

// ShouldSkip returns false — AgentCard CRs are always syncable.
func (t *Translator) ShouldSkip(_ *agenticv1.AgentCard) (bool, string) {
	return false, ""
}

// Translate converts an AgentCard CR into an AgentCardData DTO.
func (t *Translator) Translate(_ context.Context, obj *agenticv1.AgentCard) (*AgentCardData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	scopes := obj.Spec.Oauth2Scopes
	if scopes == nil {
		scopes = []string{}
	}

	return &AgentCardData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		BasePath:      obj.Spec.BasePath,
		Version:       obj.Spec.Version,
		Name:          obj.Spec.Name,
		Description:   obj.Spec.Description,
		Category:      obj.Spec.Category,
		Oauth2Scopes:  scopes,
		Specification: obj.Spec.Specification,
		Active:        obj.Status.Active,
		TeamName:      shared.TeamNameFromNamespace(obj.Namespace),
	}, nil
}

// KeyFromObject derives the composite identity key from a live AgentCard CR.
func (t *Translator) KeyFromObject(obj *agenticv1.AgentCard) AgentCardKey {
	return AgentCardKey{
		BasePath: obj.Spec.BasePath,
		TeamName: shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, uses Spec.BasePath and namespace-derived team.
// Otherwise, falls back to req.Name as basePath and namespace-derived team.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *agenticv1.AgentCard) (AgentCardKey, error) {
	if lastKnown != nil {
		return AgentCardKey{
			BasePath: lastKnown.Spec.BasePath,
			TeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return AgentCardKey{
		BasePath: req.Name,
		TeamName: shared.TeamNameFromNamespace(req.Namespace),
	}, nil
}
