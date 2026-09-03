// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription

import (
	"context"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"github.com/telekom/controlplane/projector/internal/util"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an AgenticSubscription CR to an AgenticSubscriptionData
// DTO and derives identity keys.
//
// AgenticSubscription uses a convention-based fallback delete strategy: when
// lastKnown is available, KeyFromDelete reads owner app name, team name, and
// base path from the spec. Otherwise, it falls back to using key.Name for
// both OwnerAppName and BasePath + TeamNameFromNamespace. This always
// succeeds — no ErrDeleteKeyLost.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*agenticv1.AgenticSubscription, *AgenticSubscriptionData, AgenticSubscriptionKey] = (*Translator)(nil)

// ShouldSkip returns false — AgenticSubscription CRs are always syncable.
func (t *Translator) ShouldSkip(_ *agenticv1.AgenticSubscription) (bool, string) {
	return false, ""
}

// Translate converts an AgenticSubscription CR into an
// AgenticSubscriptionData DTO. OwnerAppName comes from
// Requestor.Application.Name, OwnerTeamName from namespace. TargetBasePath =
// Spec.BasePath.
func (t *Translator) Translate(_ context.Context, obj *agenticv1.AgenticSubscription) (*AgenticSubscriptionData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	var security *model.AgenticSubscriptionSecurity
	if obj.Spec.Security != nil && obj.Spec.Security.M2M != nil {
		security = &model.AgenticSubscriptionSecurity{}
		security.M2M = &model.SubscriberMachine2MachineAuthentication{}
		if obj.Spec.Security.M2M.Client != nil {
			security.M2M.Client = util.MapAgenticOAuthToCpApi(obj.Spec.Security.M2M.Client)
		}
		if obj.Spec.Security.M2M.Basic != nil {
			security.M2M.Basic = util.MapAgenticBasicAuthToCpApi(obj.Spec.Security.M2M.Basic)
		}
		if len(obj.Spec.Security.M2M.Scopes) > 0 {
			security.M2M.Scopes = obj.Spec.Security.M2M.Scopes
		}
	}

	var traffic *model.AgenticSubscriberTraffic
	if obj.Spec.Traffic.Failover != nil {
		traffic = &model.AgenticSubscriberTraffic{
			Failover: &model.AgenticSubscriberFailover{
				Enabled: obj.Spec.Traffic.Failover.Enabled,
			},
		}
	}

	return &AgenticSubscriptionData{
		Meta:           shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:    phase,
		StatusMessage:  message,
		BasePath:       obj.Spec.BasePath,
		Security:       security,
		Traffic:        traffic,
		OwnerAppName:   obj.Spec.Requestor.Application.Name,
		OwnerTeamName:  shared.TeamNameFromNamespace(obj.Namespace),
		TargetBasePath: obj.Spec.BasePath,
	}, nil
}

// KeyFromObject derives the composite identity key from a live
// AgenticSubscription.
func (t *Translator) KeyFromObject(obj *agenticv1.AgenticSubscription) AgenticSubscriptionKey {
	return AgenticSubscriptionKey{
		BasePath:      obj.Spec.BasePath,
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
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *agenticv1.AgenticSubscription) (AgenticSubscriptionKey, error) {
	if lastKnown != nil {
		return AgenticSubscriptionKey{
			BasePath:      lastKnown.Spec.BasePath,
			OwnerAppName:  lastKnown.Spec.Requestor.Application.Name,
			OwnerTeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
			Namespace:     lastKnown.Namespace,
			Name:          lastKnown.Name,
		}, nil
	}
	return AgenticSubscriptionKey{
		BasePath:      req.Name,
		OwnerAppName:  req.Name,
		OwnerTeamName: shared.TeamNameFromNamespace(req.Namespace),
		Namespace:     req.Namespace,
		Name:          req.Name,
	}, nil
}
