// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	orgv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Translator maps a Team CR to a TeamData DTO and derives identity keys.
// Team uses the Strong delete strategy — KeyFromDelete always succeeds.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*orgv1.Team, *TeamData, TeamKey] = (*Translator)(nil)

// ShouldSkip returns false — Team CRs are always syncable.
func (t *Translator) ShouldSkip(_ *orgv1.Team) (bool, string) {
	return false, ""
}

// Translate converts a Team CR into a TeamData DTO.
// Category is upper-cased ("Customer" → "CUSTOMER") to match the ent enum.
// Status is extracted from conditions via StatusFromConditions.
// Members are mapped 1:1 from the CR spec.
func (t *Translator) Translate(_ context.Context, obj *orgv1.Team) (*TeamData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	members := make([]MemberData, len(obj.Spec.Members))
	for i, m := range obj.Spec.Members {
		members[i] = MemberData{
			Name:  m.Name,
			Email: m.Email,
		}
	}

	return &TeamData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		Name:          obj.Name,
		Email:         obj.Spec.Email,
		Category:      strings.ToUpper(string(obj.Spec.Category)),
		GroupName:     obj.Spec.Group,
		Members:       members,
	}, nil
}

// KeyFromObject derives the identity key from a live Team object.
func (t *Translator) KeyFromObject(obj *orgv1.Team) TeamKey {
	return TeamKey(obj.Name)
}

// KeyFromDelete derives the identity key for a delete operation.
// Team uses the Strong strategy — the key is always derivable from req.Name,
// which is the composite "<group>--<team>" metadata name.
func (t *Translator) KeyFromDelete(req types.NamespacedName, _ *orgv1.Team) (TeamKey, error) {
	return TeamKey(req.Name), nil
}
