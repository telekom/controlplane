// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package subscriber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/service"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestBuildSubscriptionResource_Envelope(t *testing.T) {
	subscriber := &pubsubv1.Subscriber{
		Spec: pubsubv1.SubscriberSpec{
			SubscriberId: "sub",
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:    pubsubv1.DeliveryTypeCallback,
				Payload: pubsubv1.PayloadTypeData,
			},
		},
	}
	publisher := &pubsubv1.Publisher{
		Spec: pubsubv1.PublisherSpec{EventType: "type", PublisherId: "pub"},
	}

	resource := BuildSubscriptionResource(subscriber, publisher, "sub-id-123", "playground")

	assert.Equal(t, "subscriber.horizon.telekom.de/v1", resource.ApiVersion)
	assert.Equal(t, "Subscription", resource.Kind)
	assert.Equal(t, "sub-id-123", resource.Metadata.Name)
	assert.Equal(t, service.DefaultDataplaneNamespace, resource.Metadata.Namespace)
	assert.Equal(t, "playground", resource.Spec.Environment)
}

func TestBuildSubscriptionResource_BasicFields(t *testing.T) {
	redeliveriesPerSec := 5
	subscriber := &pubsubv1.Subscriber{
		Spec: pubsubv1.SubscriberSpec{
			SubscriberId: "my-consumer-app",
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:                  pubsubv1.DeliveryTypeCallback,
				Payload:               pubsubv1.PayloadTypeData,
				Callback:              "https://my-app.example.com/events",
				EventRetentionTime:    "P7D",
				RetryableStatusCodes:  []int{502, 503},
				RedeliveriesPerSecond: &redeliveriesPerSec,
			},
			AppliedScopes: []string{"scope-a", "scope-b"},
		},
	}
	publisher := &pubsubv1.Publisher{
		Spec: pubsubv1.PublisherSpec{
			EventType:              "de.telekom.order.created.v1",
			PublisherId:            "order-service",
			AdditionalPublisherIds: []string{"order-service-v2"},
		},
	}

	resource := BuildSubscriptionResource(subscriber, publisher, "sub-id-123", "production")
	payload := resource.Spec.Subscription

	assert.Equal(t, "sub-id-123", payload.SubscriptionId)
	assert.Equal(t, "my-consumer-app", payload.SubscriberId)
	assert.Equal(t, "order-service", payload.PublisherId)
	assert.Equal(t, "de.telekom.order.created.v1", payload.Type)
	assert.Equal(t, "Callback", payload.DeliveryType)
	assert.Equal(t, "Data", payload.PayloadType)
	assert.Equal(t, "https://my-app.example.com/events", payload.Callback)
	assert.Equal(t, "P7D", payload.EventRetentionTime)
	assert.Equal(t, []int{502, 503}, payload.RetryableStatusCodes)
	require.NotNil(t, payload.RedeliveriesPerSecond)
	assert.Equal(t, 5, *payload.RedeliveriesPerSecond)
	assert.Equal(t, []string{"order-service-v2"}, payload.AdditionalPublisherIds)
	assert.Equal(t, []string{"scope-a", "scope-b"}, payload.AppliedScopes)
}

func TestBuildSubscriptionResource_CircuitBreakerOptOut(t *testing.T) {
	t.Run("true sets pointer", func(t *testing.T) {
		subscriber := &pubsubv1.Subscriber{
			Spec: pubsubv1.SubscriberSpec{
				SubscriberId: "sub",
				Delivery: pubsubv1.SubscriptionDelivery{
					Type:                 pubsubv1.DeliveryTypeCallback,
					Payload:              pubsubv1.PayloadTypeData,
					CircuitBreakerOptOut: true,
				},
			},
		}
		publisher := &pubsubv1.Publisher{
			Spec: pubsubv1.PublisherSpec{EventType: "type", PublisherId: "pub"},
		}

		resource := BuildSubscriptionResource(subscriber, publisher, "id", "env")
		payload := resource.Spec.Subscription
		require.NotNil(t, payload.CircuitBreakerOptOut)
		assert.True(t, *payload.CircuitBreakerOptOut)
	})

	t.Run("false leaves nil (NON_NULL behavior)", func(t *testing.T) {
		subscriber := &pubsubv1.Subscriber{
			Spec: pubsubv1.SubscriberSpec{
				SubscriberId: "sub",
				Delivery: pubsubv1.SubscriptionDelivery{
					Type:                 pubsubv1.DeliveryTypeCallback,
					Payload:              pubsubv1.PayloadTypeData,
					CircuitBreakerOptOut: false,
				},
			},
		}
		publisher := &pubsubv1.Publisher{
			Spec: pubsubv1.PublisherSpec{EventType: "type", PublisherId: "pub"},
		}

		resource := BuildSubscriptionResource(subscriber, publisher, "id", "env")
		assert.Nil(t, resource.Spec.Subscription.CircuitBreakerOptOut)
	})
}

func TestBuildSubscriptionResource_EnforceGetForHealthCheck(t *testing.T) {
	t.Run("true sets pointer", func(t *testing.T) {
		subscriber := &pubsubv1.Subscriber{
			Spec: pubsubv1.SubscriberSpec{
				SubscriberId: "sub",
				Delivery: pubsubv1.SubscriptionDelivery{
					Type:    pubsubv1.DeliveryTypeCallback,
					Payload: pubsubv1.PayloadTypeData,
					EnforceGetHttpRequestMethodForHealthCheck: true,
				},
			},
		}
		publisher := &pubsubv1.Publisher{
			Spec: pubsubv1.PublisherSpec{EventType: "type", PublisherId: "pub"},
		}

		resource := BuildSubscriptionResource(subscriber, publisher, "id", "env")
		payload := resource.Spec.Subscription
		require.NotNil(t, payload.EnforceGetHttpRequestMethodForHealthCheck)
		assert.True(t, *payload.EnforceGetHttpRequestMethodForHealthCheck)
	})

	t.Run("false leaves nil", func(t *testing.T) {
		subscriber := &pubsubv1.Subscriber{
			Spec: pubsubv1.SubscriberSpec{
				SubscriberId: "sub",
				Delivery: pubsubv1.SubscriptionDelivery{
					Type:    pubsubv1.DeliveryTypeCallback,
					Payload: pubsubv1.PayloadTypeData,
				},
			},
		}
		publisher := &pubsubv1.Publisher{
			Spec: pubsubv1.PublisherSpec{EventType: "type", PublisherId: "pub"},
		}

		resource := BuildSubscriptionResource(subscriber, publisher, "id", "env")
		assert.Nil(t, resource.Spec.Subscription.EnforceGetHttpRequestMethodForHealthCheck)
	})
}

func TestBuildSubscriptionResource_NilTriggers(t *testing.T) {
	subscriber := &pubsubv1.Subscriber{
		Spec: pubsubv1.SubscriberSpec{
			SubscriberId: "sub",
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:    pubsubv1.DeliveryTypeServerSentEvent,
				Payload: pubsubv1.PayloadTypeDataRef,
			},
			Trigger:          nil,
			PublisherTrigger: nil,
		},
	}
	publisher := &pubsubv1.Publisher{
		Spec: pubsubv1.PublisherSpec{EventType: "type", PublisherId: "pub"},
	}

	resource := BuildSubscriptionResource(subscriber, publisher, "id", "env")
	payload := resource.Spec.Subscription
	assert.Nil(t, payload.Trigger)
	assert.Nil(t, payload.PublisherTrigger)
}

func TestBuildSubscriptionResource_WithTrigger(t *testing.T) {
	subscriber := &pubsubv1.Subscriber{
		Spec: pubsubv1.SubscriberSpec{
			SubscriberId: "sub",
			Delivery: pubsubv1.SubscriptionDelivery{
				Type:    pubsubv1.DeliveryTypeCallback,
				Payload: pubsubv1.PayloadTypeData,
			},
			Trigger: &pubsubv1.SubscriptionTrigger{
				ResponseFilter: &pubsubv1.SubscriptionResponseFilter{
					Mode:  pubsubv1.ResponseFilterModeInclude,
					Paths: []string{"$.data.orderId", "$.data.status"},
				},
				SelectionFilter: &pubsubv1.SubscriptionSelectionFilter{
					Attributes: map[string]string{"source": "order-service"},
					Expression: &apiextensionsv1.JSON{
						Raw: []byte(`{"op":"eq","field":"type","value":"created"}`),
					},
				},
			},
		},
	}
	publisher := &pubsubv1.Publisher{
		Spec: pubsubv1.PublisherSpec{EventType: "type", PublisherId: "pub"},
	}

	resource := BuildSubscriptionResource(subscriber, publisher, "id", "env")
	payload := resource.Spec.Subscription

	require.NotNil(t, payload.Trigger)
	assert.Equal(t, "Include", payload.Trigger.ResponseFilterMode)
	assert.Equal(t, []string{"$.data.orderId", "$.data.status"}, payload.Trigger.ResponseFilter)
	assert.Equal(t, map[string]string{"source": "order-service"}, payload.Trigger.SelectionFilter)
	require.NotNil(t, payload.Trigger.AdvancedSelectionFilter)
	assert.Equal(t, "eq", payload.Trigger.AdvancedSelectionFilter["op"])
	assert.Equal(t, "type", payload.Trigger.AdvancedSelectionFilter["field"])
	assert.Equal(t, "created", payload.Trigger.AdvancedSelectionFilter["value"])
}

func TestConvertTrigger_Nil(t *testing.T) {
	result := convertTrigger(nil)
	assert.Nil(t, result)
}

func TestConvertTrigger_EmptyTrigger(t *testing.T) {
	trigger := &pubsubv1.SubscriptionTrigger{}
	result := convertTrigger(trigger)
	require.NotNil(t, result)
	assert.Empty(t, result.ResponseFilterMode)
	assert.Nil(t, result.ResponseFilter)
	assert.Nil(t, result.SelectionFilter)
	assert.Nil(t, result.AdvancedSelectionFilter)
}

func TestConvertTrigger_InvalidExpressionJSON(t *testing.T) {
	trigger := &pubsubv1.SubscriptionTrigger{
		SelectionFilter: &pubsubv1.SubscriptionSelectionFilter{
			Expression: &apiextensionsv1.JSON{
				Raw: []byte(`invalid json`),
			},
		},
	}
	result := convertTrigger(trigger)
	require.NotNil(t, result)
	// Invalid JSON should result in nil AdvancedSelectionFilter (silently ignored)
	assert.Nil(t, result.AdvancedSelectionFilter)
}

func TestGetOrGenerateSubscriptionID_PrefersStatus(t *testing.T) {
	obj := &pubsubv1.Subscriber{
		Spec: pubsubv1.SubscriberSpec{SubscriberId: "sub"},
		Status: pubsubv1.SubscriberStatus{
			SubscriptionId: "existing-id-from-status",
		},
	}
	result := getOrGenerateSubscriptionID(obj, "env", "type")
	assert.Equal(t, "existing-id-from-status", result)
}

func TestGetOrGenerateSubscriptionID_GeneratesWhenEmpty(t *testing.T) {
	obj := &pubsubv1.Subscriber{
		Spec:   pubsubv1.SubscriberSpec{SubscriberId: "sub"},
		Status: pubsubv1.SubscriberStatus{},
	}
	result := getOrGenerateSubscriptionID(obj, "env", "type")
	expected := GenerateSubscriptionID("env", "type", "sub")
	assert.Equal(t, expected, result)
	assert.Len(t, result, 40) // SHA-1 hex length
}
