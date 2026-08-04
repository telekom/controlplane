// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// applicationLabelKey is the label key used by the Rover controller to
// associate a PermissionSet CR with its owner Application.
const applicationLabelKey = "cp.ei.telekom.de/application"

// Translator maps a PermissionSet CR to a PermissionSetData DTO and derives
// identity keys.
//
// ShouldSkip filters out CRs that lack the application label — AppName is
// the identity key used for both the DB FK lookup and delete, so it must
// never be empty.
//
// KeyFromDelete uses lastKnown when available. When lastKnown is nil,
// returns ErrDeleteKeyLost: the owning Application name cannot be safely
// derived from req.Name (the PermissionSet CR's own name), since there is no
// guarantee the two match.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*permissionv1.PermissionSet, *PermissionSetData, PermissionSetKey] = (*Translator)(nil)

// ShouldSkip returns true if the PermissionSet CR lacks the application
// label needed to resolve its owner Application.
func (t *Translator) ShouldSkip(obj *permissionv1.PermissionSet) (bool, string) {
	if obj.Labels[applicationLabelKey] == "" {
		return true, "cp.ei.telekom.de/application label is missing or empty"
	}
	return false, ""
}

// Translate converts a PermissionSet CR into a PermissionSetData DTO.
// Permissions are mapped 1:1 from spec.permissions. AppName comes from the
// application label, TeamName from the namespace.
func (t *Translator) Translate(_ context.Context, obj *permissionv1.PermissionSet) (*PermissionSetData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	permissions := make([]model.Permission, len(obj.Spec.Permissions))
	for i, p := range obj.Spec.Permissions {
		permissions[i] = model.Permission{
			Role:     p.Role,
			Resource: p.Resource,
			Actions:  p.Actions,
		}
	}

	return &PermissionSetData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		Permissions:   permissions,
		AppName:       obj.Labels[applicationLabelKey],
		TeamName:      shared.TeamNameFromNamespace(obj.Namespace),
	}, nil
}

// KeyFromObject derives the composite identity key from a live PermissionSet.
func (t *Translator) KeyFromObject(obj *permissionv1.PermissionSet) PermissionSetKey {
	return PermissionSetKey{
		AppName:  obj.Labels[applicationLabelKey],
		TeamName: shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, AppName comes from the application label and
// TeamName from the namespace. If lastKnown is nil (cache miss), returns
// ErrDeleteKeyLost — the owning Application name cannot be safely derived
// from req.Name, since the PermissionSet CR's own name is not guaranteed to
// match its owner Application's name.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *permissionv1.PermissionSet) (PermissionSetKey, error) {
	if lastKnown != nil {
		return PermissionSetKey{
			AppName:  lastKnown.Labels[applicationLabelKey],
			TeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return PermissionSetKey{}, fmt.Errorf("permissionset %s/%s: %w", req.Namespace, req.Name, runtime.ErrDeleteKeyLost)
}
