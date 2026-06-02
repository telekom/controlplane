// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype

import (
	"context"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an EventType CR to an EventTypeData DTO and derives
// identity keys.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*eventv1.EventType, *EventTypeData, EventTypeKey] = (*Translator)(nil)

// ShouldSkip returns false — EventType CRs are always syncable.
func (t *Translator) ShouldSkip(_ *eventv1.EventType) (bool, string) {
	return false, ""
}

// Translate converts an EventType CR into an EventTypeData DTO.
func (t *Translator) Translate(_ context.Context, obj *eventv1.EventType) (*EventTypeData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	return &EventTypeData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		EventType:     obj.Spec.Type,
		Version:       obj.Spec.Version,
		Description:   obj.Spec.Description,
		Specification: obj.Spec.Specification,
		Active:        obj.Status.Active,
		TeamName:      shared.TeamNameFromNamespace(obj.Namespace),
	}, nil
}

// KeyFromObject derives the composite identity key from a live EventType CR.
func (t *Translator) KeyFromObject(obj *eventv1.EventType) EventTypeKey {
	return EventTypeKey{
		EventType: obj.Spec.Type,
		TeamName:  shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, uses Spec.Type and namespace-derived team.
// Otherwise, falls back to req.Name as event type and namespace-derived team.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *eventv1.EventType) (EventTypeKey, error) {
	if lastKnown != nil {
		return EventTypeKey{
			EventType: lastKnown.Spec.Type,
			TeamName:  shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return EventTypeKey{
		EventType: req.Name,
		TeamName:  shared.TeamNameFromNamespace(req.Namespace),
	}, nil
}
