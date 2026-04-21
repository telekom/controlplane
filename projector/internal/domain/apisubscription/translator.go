// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an ApiSubscription CR to an APISubscriptionData DTO and
// derives identity keys.
//
// ApiSubscription uses a convention-based fallback delete strategy: when
// lastKnown is available, KeyFromDelete reads owner app name, team name, and
// base path from the spec. Otherwise, it falls back to using key.Name for
// both OwnerAppName and BasePath + TeamNameFromNamespace. This always succeeds
// — no ErrDeleteKeyLost. (In practice, without lastKnown, the best-effort key
// is unlikely to match, making the entity an orphan for GC.)
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*apiv1.ApiSubscription, *APISubscriptionData, APISubscriptionKey] = (*Translator)(nil)

// ShouldSkip returns false — ApiSubscription CRs are always syncable.
func (t *Translator) ShouldSkip(_ *apiv1.ApiSubscription) (bool, string) {
	return false, ""
}

// Translate converts an ApiSubscription CR into an APISubscriptionData DTO.
//
// M2MAuthMethod is derived from the Security spec:
//
//	security == nil     → "NONE"
//	security.M2M == nil → "NONE"
//	m2m.Client != nil   → "OAUTH2_CLIENT"
//	m2m.Basic != nil    → "BASIC_AUTH"
//	len(m2m.Scopes) > 0 → "SCOPES_ONLY"
//	otherwise           → "NONE"
//
// ApprovedScopes comes from security.M2M.Scopes, defaulting to an empty slice.
// OwnerAppName from Requestor.Application.Name, OwnerTeamName from namespace.
// TargetBasePath = Spec.ApiBasePath. TargetAppName/TargetTeamName are always "".
func (t *Translator) Translate(_ context.Context, obj *apiv1.ApiSubscription) (*APISubscriptionData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	return &APISubscriptionData{
		Meta:           shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:    phase,
		StatusMessage:  message,
		BasePath:       obj.Spec.ApiBasePath,
		M2MAuthMethod:  deriveM2MAuthMethod(obj.Spec.Security),
		ApprovedScopes: deriveApprovedScopes(obj.Spec.Security),
		OwnerAppName:   obj.Spec.Requestor.Application.Name,
		OwnerTeamName:  shared.TeamNameFromNamespace(obj.Namespace),
		TargetBasePath: obj.Spec.ApiBasePath,
		TargetAppName:  "", // TODO: this needs to be improved, we need to get the ApiExposure into the context to resolve this
		TargetTeamName: "",
	}, nil
}

// KeyFromObject derives the composite identity key from a live ApiSubscription.
func (t *Translator) KeyFromObject(obj *apiv1.ApiSubscription) APISubscriptionKey {
	return APISubscriptionKey{
		BasePath:      obj.Spec.ApiBasePath,
		OwnerAppName:  obj.Spec.Requestor.Application.Name,
		OwnerTeamName: shared.TeamNameFromNamespace(obj.Namespace),
		Namespace:     obj.Namespace,
		Name:          obj.Name,
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, all fields are taken from the spec + metadata.
// Otherwise, key.Name is used as best-effort for both OwnerAppName and
// BasePath, and OwnerTeamName is derived from the namespace convention.
// Always succeeds (no ErrDeleteKeyLost).
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *apiv1.ApiSubscription) (APISubscriptionKey, error) {
	if lastKnown != nil {
		return APISubscriptionKey{
			BasePath:      lastKnown.Spec.ApiBasePath,
			OwnerAppName:  lastKnown.Spec.Requestor.Application.Name,
			OwnerTeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
			Namespace:     lastKnown.Namespace,
			Name:          lastKnown.Name,
		}, nil
	}
	return APISubscriptionKey{
		BasePath:      req.Name,
		OwnerAppName:  req.Name,
		OwnerTeamName: shared.TeamNameFromNamespace(req.Namespace),
		Namespace:     req.Namespace,
		Name:          req.Name,
	}, nil
}

// deriveM2MAuthMethod maps the CR's security configuration to the ent enum
// value for the m2m_auth_method field.
func deriveM2MAuthMethod(security *apiv1.SubscriberSecurity) string {
	if security == nil || security.M2M == nil {
		return "NONE"
	}
	m2m := security.M2M
	if m2m.Client != nil {
		return "OAUTH2_CLIENT"
	}
	if m2m.Basic != nil {
		return "BASIC_AUTH"
	}
	if len(m2m.Scopes) > 0 {
		return "SCOPES_ONLY"
	}
	return "NONE"
}

// deriveApprovedScopes extracts the M2M scopes from the security config. If
// security or M2M is nil, returns an empty slice (never nil).
func deriveApprovedScopes(security *apiv1.SubscriberSecurity) []string {
	if security == nil || security.M2M == nil || len(security.M2M.Scopes) == 0 {
		return []string{}
	}
	return security.M2M.Scopes
}
