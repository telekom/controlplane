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
		DeliveryType:           delivery.Type.String(),
		PayloadType:            delivery.Payload.String(),
		Callback:               delivery.Callback,
		AdditionalPublisherIds: publisher.Spec.AdditionalPublisherIds,
		AppliedScopes:          spec.AppliedScopes,
		EventRetentionTime:     delivery.EventRetentionTime,
		RetryableStatusCodes:   delivery.RetryableStatusCodes,
		RedeliveriesPerSecond:  delivery.RedeliveriesPerSecond,
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
func convertTrigger(trigger *pubsubv1.SubscriptionTrigger) *service.SubscriptionTriggerPayload {
	if trigger == nil {
		return nil
	}

	result := &service.SubscriptionTriggerPayload{}

	if trigger.ResponseFilter != nil {
		result.ResponseFilterMode = trigger.ResponseFilter.Mode.String()
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
