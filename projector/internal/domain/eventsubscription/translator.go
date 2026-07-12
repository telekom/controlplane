// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"context"
	"strings"

	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"github.com/telekom/controlplane/projector/internal/util"
	"k8s.io/apimachinery/pkg/types"
)

// applicationLabelKey is the label key used by the Rover controller to
// associate an EventSubscription CR with its owner Application.
const applicationLabelKey = "cp.ei.telekom.de/application"

// Translator maps an EventSubscription CR to an EventSubscriptionData DTO
// and derives identity keys.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*eventv1.EventSubscription, *EventSubscriptionData, EventSubscriptionKey] = (*Translator)(nil)

// ShouldSkip returns false — EventSubscription CRs are always syncable.
func (t *Translator) ShouldSkip(_ *eventv1.EventSubscription) (bool, string) {
	return false, ""
}

// Translate converts an EventSubscription CR into an EventSubscriptionData DTO.
// DeliveryType is upper-cased with underscores (Callback→CALLBACK,
// ServerSentEvent→SERVER_SENT_EVENT). CallbackURL is set when delivery type
// is Callback. OwnerAppName from Requestor.Application.Name, OwnerTeamName
// from namespace.
func (t *Translator) Translate(_ context.Context, obj *eventv1.EventSubscription) (*EventSubscriptionData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	var callbackURL *string
	if obj.Spec.Delivery.Callback != "" {
		s := obj.Spec.Delivery.Callback
		callbackURL = &s
	}

	var trigger *model.EventTrigger
	if obj.Spec.Trigger != nil {
		t := util.MapEventTrigger(*obj.Spec.Trigger.DeepCopy())
		trigger = &t
	}

	return &EventSubscriptionData{
		Meta:            shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:     phase,
		StatusMessage:   message,
		EventType:       obj.Spec.EventType,
		DeliveryType:    mapDeliveryType(string(obj.Spec.Delivery.Type)),
		Trigger:         trigger,
		Delivery:        mapDelivery(obj.Spec.Delivery),
		Scopes:          obj.Spec.Scopes,
		CallbackURL:     callbackURL,
		OwnerAppName:    obj.Spec.Requestor.Name,
		OwnerTeamName:   shared.TeamNameFromNamespace(obj.Namespace),
		TargetEventType: obj.Spec.EventType,
	}, nil
}

func mapDelivery(obj eventv1.Delivery) *model.EventDelivery {
	delivery := &model.EventDelivery{
		Payload:               obj.Payload.String(),
		EventRetentionTime:    obj.EventRetentionTime,
		CircuitBreakerOptOut:  obj.CircuitBreakerOptOut,
		RetryableStatusCodes:  obj.RetryableStatusCodes,
		RedeliveriesPerSecond: obj.RedeliveriesPerSecond,
		EnforceGetHttpRequestMethodForHealthCheck: obj.EnforceGetHttpRequestMethodForHealthCheck,
	}

	return delivery
}

// KeyFromObject derives the composite identity key from a live EventSubscription.
func (t *Translator) KeyFromObject(obj *eventv1.EventSubscription) EventSubscriptionKey {
	return EventSubscriptionKey{
		EventType:     obj.Spec.EventType,
		OwnerAppName:  obj.Spec.Requestor.Name,
		OwnerTeamName: shared.TeamNameFromNamespace(obj.Namespace),
		Namespace:     obj.Namespace,
		Name:          obj.Name,
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, all fields are taken from the spec + metadata.
// Otherwise, key.Name is used as best-effort. Always succeeds.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *eventv1.EventSubscription) (EventSubscriptionKey, error) {
	if lastKnown != nil {
		return EventSubscriptionKey{
			EventType:     lastKnown.Spec.EventType,
			OwnerAppName:  lastKnown.Spec.Requestor.Name,
			OwnerTeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
			Namespace:     lastKnown.Namespace,
			Name:          lastKnown.Name,
		}, nil
	}
	return EventSubscriptionKey{
		EventType:     req.Name,
		OwnerAppName:  req.Name,
		OwnerTeamName: shared.TeamNameFromNamespace(req.Namespace),
		Namespace:     req.Namespace,
		Name:          req.Name,
	}, nil
}

// mapDeliveryType converts CR delivery type values to the DB enum
// representation. CR uses PascalCase (Callback, ServerSentEvent), while
// the DB uses uppercase with underscores (CALLBACK, SERVER_SENT_EVENT).
func mapDeliveryType(dt string) string {
	switch dt {
	case "Callback":
		return "CALLBACK"
	case "ServerSentEvent":
		return "SERVER_SENT_EVENT"
	default:
		return strings.ToUpper(dt)
	}
}
