// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an Api CR to an ApiData DTO and derives identity keys.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*apiv1.Api, *ApiData, ApiKey] = (*Translator)(nil)

// ShouldSkip returns false — Api CRs are always syncable.
func (t *Translator) ShouldSkip(_ *apiv1.Api) (bool, string) {
	return false, ""
}

// Translate converts an Api CR into an ApiData DTO.
func (t *Translator) Translate(_ context.Context, obj *apiv1.Api) (*ApiData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	scopes := obj.Spec.Oauth2Scopes
	if scopes == nil {
		scopes = []string{}
	}

	return &ApiData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		BasePath:      obj.Spec.BasePath,
		Version:       obj.Spec.Version,
		Category:      obj.Spec.Category,
		Oauth2Scopes:  scopes,
		XVendor:       obj.Spec.XVendor,
		Specification: obj.Spec.Specification,
		Active:        obj.Status.Active,
		TeamName:      shared.TeamNameFromNamespace(obj.Namespace),
	}, nil
}

// KeyFromObject derives the composite identity key from a live Api CR.
func (t *Translator) KeyFromObject(obj *apiv1.Api) ApiKey {
	return ApiKey{
		BasePath: obj.Spec.BasePath,
		TeamName: shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, uses Spec.BasePath and namespace-derived team.
// Otherwise, falls back to req.Name as basePath and namespace-derived team.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *apiv1.Api) (ApiKey, error) {
	if lastKnown != nil {
		return ApiKey{
			BasePath: lastKnown.Spec.BasePath,
			TeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return ApiKey{
		BasePath: req.Name,
		TeamName: shared.TeamNameFromNamespace(req.Namespace),
	}, nil
}
