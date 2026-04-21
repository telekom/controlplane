// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package subscriber

import (
	"encoding/json"

	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/service"
)

// BuildSubscriptionResource maps a Subscriber spec and its resolved Publisher
// into a full SubscriptionResource suitable for the configuration backend REST API.
// The configuration backend expects a Kubernetes-style resource with apiVersion, kind,
// metadata, and spec fields.
func BuildSubscriptionResource(subscriber *pubsubv1.Subscriber, publisher *pubsubv1.Publisher, subscriptionID, environment string) service.SubscriptionResource {
	spec := subscriber.Spec
	delivery := spec.Delivery

	payload := service.SubscriptionPayload{
		SubscriptionId:         subscriptionID,
		SubscriberId:           spec.SubscriberId,
		PublisherId:            publisher.Spec.PublisherId,
		Type:                   publisher.Spec.EventType,
		DeliveryType:           convertDeliveryType(delivery.Type),
		PayloadType:            convertPayloadType(delivery.Payload),
		Callback:               delivery.Callback,
		AdditionalPublisherIds: publisher.Spec.AdditionalPublisherIds,
		AppliedScopes:          spec.AppliedScopes,
		EventRetentionTime:     delivery.EventRetentionTime,
		RetryableStatusCodes:   delivery.RetryableStatusCodes,
		RedeliveriesPerSecond:  delivery.RedeliveriesPerSecond,
		JsonSchema:             publisher.Spec.JsonSchema,
	}

	if delivery.CircuitBreakerOptOut {
		payload.CircuitBreakerOptOut = &delivery.CircuitBreakerOptOut
	}
	if delivery.EnforceGetHttpRequestMethodForHealthCheck {
		payload.EnforceGetHttpRequestMethodForHealthCheck = &delivery.EnforceGetHttpRequestMethodForHealthCheck
	}

	payload.Trigger = convertTrigger(spec.Trigger)
	payload.PublisherTrigger = convertTrigger(spec.PublisherTrigger)

	return service.SubscriptionResource{
		ApiVersion: service.SubscriptionAPIVersion,
		Kind:       service.SubscriptionKind,
		Metadata: service.SubscriptionMetadata{
			Name:      subscriptionID,
			Namespace: service.DefaultDataplaneNamespace,
		},
		Spec: service.SubscriptionSpec{
			Environment:  environment,
			Subscription: payload,
		},
	}
}

// convertTrigger maps a K8s SubscriptionTrigger to a client SubscriptionTriggerPayload.
// Returns nil if the input is nil.
func convertTrigger(trigger *pubsubv1.Trigger) *service.SubscriptionTriggerPayload {
	if trigger == nil {
		return nil
	}

	result := &service.SubscriptionTriggerPayload{}

	if trigger.ResponseFilter != nil {
		result.ResponseFilterMode = convertResponseFilterMode(trigger.ResponseFilter.Mode)
		result.ResponseFilter = trigger.ResponseFilter.Paths
	}

	if trigger.SelectionFilter != nil {
		result.SelectionFilter = trigger.SelectionFilter.Attributes

		if trigger.SelectionFilter.Expression != nil {
			var advancedFilter map[string]any
			if err := json.Unmarshal(trigger.SelectionFilter.Expression.Raw, &advancedFilter); err == nil {
				result.AdvancedSelectionFilter = advancedFilter
			}
		}
	}

	return result
}

func convertDeliveryType(deliveryType pubsubv1.DeliveryType) string {
	switch deliveryType {
	case pubsubv1.DeliveryTypeCallback:
		return "callback"
	case pubsubv1.DeliveryTypeServerSentEvent:
		return "server_sent_event"
	default:
		panic("unknown delivery type")
	}
}

func convertPayloadType(payloadType pubsubv1.PayloadType) string {
	switch payloadType {
	case pubsubv1.PayloadTypeData:
		return "data"
	case pubsubv1.PayloadTypeDataRef:
		return "data_ref"
	default:
		panic("unknown payload type")
	}
}

func convertResponseFilterMode(mode pubsubv1.ResponseFilterMode) string {
	switch mode {
	case pubsubv1.ResponseFilterModeInclude:
		return "INCLUDE"
	case pubsubv1.ResponseFilterModeExclude:
		return "EXCLUDE"
	default:
		panic("unknown response filter mode")
	}
}
