// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"time"

	appv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

// Secret rotation phase constants.
const (
	RotationPhaseDone        = "DONE"
	RotationPhaseInProgress  = "IN_PROGRESS"
	RotationPhaseGracePeriod = "GRACE_PERIOD"
	RotationPhaseFailed      = "FAILED"

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
	}, nil
}

// deriveRotationState maps the SecretRotation condition to an FSM phase.
//
// Rules:
//  1. Condition absent → DONE
//  2. Reason "InProgress" → IN_PROGRESS
//  3. Reason "Success" + RotatedClientSecret non-empty → GRACE_PERIOD
//  4. Reason "Success" + RotatedClientSecret empty → DONE
//  5. Reason "Failed" or "Error" → FAILED
//  6. Unknown reason → IN_PROGRESS (safe fallback)
func deriveRotationState(obj *appv1.Application) (phase string, message *string) {
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
		return RotationPhaseInProgress, msg
	case "Success":
		if obj.Status.RotatedClientSecret != "" {
			return RotationPhaseGracePeriod, msg
		}
		return RotationPhaseDone, nil
	case "Failed", "Error":
		return RotationPhaseFailed, msg
	default:
		return RotationPhaseInProgress, msg
	}
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
