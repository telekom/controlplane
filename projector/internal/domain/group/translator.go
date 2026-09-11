// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"

	orgv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps a Group CR to a GroupData DTO and derives identity keys.
// Group uses the Strong delete strategy — KeyFromDelete always succeeds.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*orgv1.Group, *GroupData, GroupKey] = (*Translator)(nil)

// ShouldSkip returns false — Group CRs are always syncable.
func (t *Translator) ShouldSkip(_ *orgv1.Group) (bool, string) {
	return false, ""
}

// Translate converts a Group CR into a GroupData DTO.
// Maps Name, DisplayName, and Description from the CR spec.
func (t *Translator) Translate(_ context.Context, obj *orgv1.Group) (*GroupData, error) {
	return &GroupData{
		Meta:        shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		Name:        obj.Name,
		DisplayName: obj.Spec.DisplayName,
		Description: obj.Spec.Description,
	}, nil
}

// KeyFromObject derives the identity key from a live Group object.
func (t *Translator) KeyFromObject(obj *orgv1.Group) GroupKey {
	return GroupKey(obj.Name)
}

// KeyFromDelete derives the identity key for a delete operation.
// Group uses the Strong strategy — the key is always derivable from req.Name.
func (t *Translator) KeyFromDelete(req types.NamespacedName, _ *orgv1.Group) (GroupKey, error) {
	return GroupKey(req.Name), nil
}
