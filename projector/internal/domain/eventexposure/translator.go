// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	"context"
	"strings"

	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// applicationLabelKey is the label key used by the Rover controller to
// associate an EventExposure CR with its owner Application.
const applicationLabelKey = "cp.ei.telekom.de/application"

// Translator maps an EventExposure CR to an EventExposureData DTO and derives
// identity keys.
//
// EventExposure uses a convention-based fallback delete strategy: when
// lastKnown is available, KeyFromDelete reads Spec.EventType + label for
// app name + namespace for team name. Otherwise, it falls back to using
// key.Name for both eventType and appName + TeamNameFromNamespace.
// This always succeeds — no ErrDeleteKeyLost.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*eventv1.EventExposure, *EventExposureData, EventExposureKey] = (*Translator)(nil)

// ShouldSkip returns false — EventExposure CRs are always syncable.
func (t *Translator) ShouldSkip(_ *eventv1.EventExposure) (bool, string) {
	return false, ""
}

// Translate converts an EventExposure CR into an EventExposureData DTO.
// Visibility is upper-cased (World→WORLD, Zone→ZONE, Enterprise→ENTERPRISE).
// Active comes from Status.Active. ApprovalConfig strategy is mapped from
// PascalCase to DB enum (Auto→AUTO, Simple→SIMPLE, FourEyes→FOUR_EYES).
// AppName comes from the application label, TeamName from namespace.
func (t *Translator) Translate(_ context.Context, obj *eventv1.EventExposure) (*EventExposureData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	return &EventExposureData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		EventType:     obj.Spec.EventType,
		Visibility:    strings.ToUpper(string(obj.Spec.Visibility)),
		Active:        obj.Status.Active,
		ApprovalConfig: model.ApprovalConfig{
			Strategy:     mapApprovalStrategy(string(obj.Spec.Approval.Strategy)),
			TrustedTeams: obj.Spec.Approval.TrustedTeams,
		},
		Scopes:   mapEventScopes(obj.Spec.Scopes),
		AppName:  obj.Labels[applicationLabelKey],
		TeamName: shared.TeamNameFromNamespace(obj.Namespace),
	}, nil
}

func mapEventScopes(eventScopes []eventv1.EventScope) []model.EventScope {
	scopes := []model.EventScope{}

	for i := range eventScopes {
		scope := model.EventScope{}

		scope.Name = eventScopes[i].Name
		scope.Trigger = model.EventTrigger{}

		if eventScopes[i].Trigger.ResponseFilter != nil {
			scope.Trigger.ResponseFilter = &model.ResponseFilter{
				Paths: eventScopes[i].Trigger.ResponseFilter.Paths,
				Mode:  eventScopes[i].Trigger.ResponseFilter.Mode.String(),
			}
		}

		if eventScopes[i].Trigger.SelectionFilter != nil {
			sf := &model.SelectionFilter{
				Attributes: eventScopes[i].Trigger.SelectionFilter.Attributes,
			}
			if eventScopes[i].Trigger.SelectionFilter.Expression != nil {
				sf.Expression = string(eventScopes[i].Trigger.SelectionFilter.Expression.Raw)
			}
			scope.Trigger.SelectionFilter = sf
		}

		scopes = append(scopes, scope)
	}

	return scopes
}

// KeyFromObject derives the composite identity key from a live EventExposure.
func (t *Translator) KeyFromObject(obj *eventv1.EventExposure) EventExposureKey {
	return EventExposureKey{
		EventType: obj.Spec.EventType,
		AppName:   obj.Labels[applicationLabelKey],
		TeamName:  shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, eventType comes from Spec.EventType, appName
// from the application label, and teamName from the namespace. Otherwise,
// key.Name is used as best-effort for both eventType and appName, and teamName
// is derived from the namespace convention. Always succeeds.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *eventv1.EventExposure) (EventExposureKey, error) {
	if lastKnown != nil {
		return EventExposureKey{
			EventType: lastKnown.Spec.EventType,
			AppName:   lastKnown.Labels[applicationLabelKey],
			TeamName:  shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return EventExposureKey{
		EventType: req.Name,
		AppName:   req.Name,
		TeamName:  shared.TeamNameFromNamespace(req.Namespace),
	}, nil
}

// mapApprovalStrategy converts CR approval strategy values to the DB enum
// representation. CR uses PascalCase (Auto, Simple, FourEyes), while the DB
// uses uppercase with underscores (AUTO, SIMPLE, FOUR_EYES).
func mapApprovalStrategy(strategy string) string {
	switch strategy {
	case "Auto":
		return "AUTO"
	case "Simple":
		return "SIMPLE"
	case "FourEyes":
		return "FOUR_EYES"
	default:
		return strings.ToUpper(strategy)
	}
}
