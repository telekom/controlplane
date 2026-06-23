// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"time"

	appv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

// Secret rotation phase constants.
const (
	RotationPhaseDone                = "DONE"
	RotationPhaseRotating            = "ROTATING"
	RotationPhaseGracePeriodActive   = "GRACE_PERIOD_ACTIVE"
	RotationPhaseGracePeriodExpiring = "GRACE_PERIOD_EXPIRING"
	RotationPhaseFailed              = "FAILED"

	// gracePeriodExpiringThreshold is the fraction of the total grace period
	// below which the phase switches from GRACE_PERIOD_ACTIVE to
	// GRACE_PERIOD_EXPIRING (20%).
	gracePeriodExpiringThreshold = 0.2

	// secretRotationConditionType is the condition type on the Application CR.
	secretRotationConditionType = "SecretRotation"
)

// Translator maps an Application CR to an ApplicationData DTO and derives
// identity keys.
//
// Application uses a convention-based fallback delete strategy: when lastKnown
// is available KeyFromDelete reads Spec.Team; otherwise it falls back to
// TeamNameFromNamespace. This always succeeds — no ErrDeleteKeyLost.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*appv1.Application, *ApplicationData, ApplicationKey] = (*Translator)(nil)

// ShouldSkip returns false — Application CRs are always syncable.
func (t *Translator) ShouldSkip(_ *appv1.Application) (bool, string) {
	return false, ""
}

// Translate converts an Application CR into an ApplicationData DTO.
// ClientID is nil when Status.ClientId is empty (populated asynchronously by
// the identity controller).
func (t *Translator) Translate(_ context.Context, obj *appv1.Application) (*ApplicationData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	var clientID *string
	if obj.Status.ClientId != "" {
		clientID = &obj.Status.ClientId
	}

	var clientSecret *string
	if obj.Spec.Secret != "" {
		clientSecret = &obj.Spec.Secret
	}

	// Secret rotation fields
	rotationPhase, rotationMessage := deriveRotationState(obj)

	var rotatedClientSecret *string
	if obj.Status.RotatedClientSecret != "" {
		rotatedClientSecret = &obj.Status.RotatedClientSecret
	}

	var rotatedExpiresAt *time.Time
	if obj.Status.RotatedExpiresAt != nil {
		t := obj.Status.RotatedExpiresAt.Time
		rotatedExpiresAt = &t
	}

	var currentExpiresAt *time.Time
	if obj.Status.CurrentExpiresAt != nil {
		t := obj.Status.CurrentExpiresAt.Time
		currentExpiresAt = &t
	}

	var externalIds []model.ExternalId
	if len(obj.Spec.ExternalIds) > 0 {
		externalIds = []model.ExternalId{}
		for i := range obj.Spec.ExternalIds {
			externalIds = append(externalIds, model.ExternalId{
				Id:     obj.Spec.ExternalIds[i].Id,
				Scheme: obj.Spec.ExternalIds[i].Scheme,
			},
			)
		}
	}

	var ipRestrictionsAllow []string
	var ipRestrictionsDeny []string
	if obj.Spec.Security != nil && obj.Spec.Security.IpRestrictions != nil {
		if len(obj.Spec.Security.IpRestrictions.Allow) > 0 {
			ipRestrictionsAllow = []string{}
			for i := range obj.Spec.Security.IpRestrictions.Allow {
				ipRestrictionsAllow = append(ipRestrictionsAllow, obj.Spec.Security.IpRestrictions.Allow[i])
			}
		}

		if len(obj.Spec.Security.IpRestrictions.Deny) > 0 {
			ipRestrictionsDeny = []string{}
			for i := range obj.Spec.Security.IpRestrictions.Deny {
				ipRestrictionsDeny = append(ipRestrictionsDeny, obj.Spec.Security.IpRestrictions.Deny[i])
			}
		}
	}

	return &ApplicationData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		Name:          obj.Name,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		TeamName:      obj.Spec.Team,
		ZoneName:      obj.Spec.Zone.Name,

		RotatedClientSecret:   rotatedClientSecret,
		RotatedExpiresAt:      rotatedExpiresAt,
		CurrentExpiresAt:      currentExpiresAt,
		SecretRotationPhase:   rotationPhase,
		SecretRotationMessage: rotationMessage,
		ExternalIds:           externalIds,
		IpRestrictions: model.IpRestrictions{
			Allow: ipRestrictionsAllow,
			Deny:  ipRestrictionsDeny,
		},
	}, nil
}

// deriveRotationState maps the SecretRotation condition to an FSM phase.
//
// Rules:
//  1. Condition absent → DONE
//  2. Reason "InProgress" → ROTATING
//  3. Reason "Success" + RotatedClientSecret non-empty → GRACE_PERIOD_ACTIVE or GRACE_PERIOD_EXPIRING
//  4. Reason "Success" + RotatedClientSecret empty → DONE
//  5. Reason "Failed" or "Error" → FAILED
//  6. Unknown reason → ROTATING (safe fallback)
func deriveRotationState(obj *appv1.Application) (phase string, message *string) {
	return deriveRotationStateAt(obj, time.Now())
}

// deriveRotationStateAt is the time-parameterised implementation of
// deriveRotationState, allowing deterministic testing.
func deriveRotationStateAt(obj *appv1.Application, now time.Time) (phase string, message *string) {
	cond := meta.FindStatusCondition(obj.Status.Conditions, secretRotationConditionType)
	if cond == nil {
		return RotationPhaseDone, nil
	}

	var msg *string
	if cond.Message != "" {
		msg = &cond.Message
	}

	switch cond.Reason {
	case "InProgress":
		return RotationPhaseRotating, msg
	case "Success":
		if obj.Status.RotatedExpiresAt != nil && obj.Status.RotatedExpiresAt.Time.After(now) {
			return gracePeriodPhase(cond.LastTransitionTime.Time, obj.Status.RotatedExpiresAt.Time, now), msg
		}
		return RotationPhaseDone, nil
	case "Failed", "Error":
		return RotationPhaseFailed, msg
	default:
		return RotationPhaseRotating, msg
	}
}

// gracePeriodPhase returns GRACE_PERIOD_EXPIRING when less than 20% of the
// total grace period remains, otherwise GRACE_PERIOD_ACTIVE.
//
// Precondition: rotatedExpiresAt is non-nil and in the future relative to now.
// The function is still defensive against edge cases (zero-length period, etc.).
func gracePeriodPhase(gracePeriodStart time.Time, expiresAt time.Time, now time.Time) string {
	total := expiresAt.Sub(gracePeriodStart)
	if total <= 0 {
		return RotationPhaseGracePeriodExpiring
	}

	remaining := expiresAt.Sub(now)
	if float64(remaining)/float64(total) < gracePeriodExpiringThreshold {
		return RotationPhaseGracePeriodExpiring
	}
	return RotationPhaseGracePeriodActive
}

// KeyFromObject derives the composite identity key from a live Application.
func (t *Translator) KeyFromObject(obj *appv1.Application) ApplicationKey {
	return ApplicationKey{
		Name:     obj.Name,
		TeamName: obj.Spec.Team,
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, the team name comes from Spec.Team.
// Otherwise, it falls back to extracting the team name from the namespace
// convention ("<env>--<group>--<team>"). Always succeeds.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *appv1.Application) (ApplicationKey, error) {
	teamName := shared.TeamNameFromNamespace(req.Namespace)
	if lastKnown != nil {
		teamName = lastKnown.Spec.Team
	}
	return ApplicationKey{
		Name:     req.Name,
		TeamName: teamName,
	}, nil
}
