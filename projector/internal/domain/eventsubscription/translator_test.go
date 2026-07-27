// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/domain/eventsubscription"
)

var _ = Describe("EventSubscription Translator", func() {
	var t eventsubscription.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			obj := &eventv1.EventSubscription{}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR with Callback delivery", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-subscription",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
						"cp.ei.telekom.de/application": "my-app",
					},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.eni.quickstart.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "consumer-app"},
					},
					Delivery: eventv1.Delivery{
						Type:                  eventv1.DeliveryTypeCallback,
						Payload:               eventv1.PayloadTypeData,
						Callback:              "https://consumer.example.com/events",
						EventRetentionTime:    "7d",
						CircuitBreakerOptOut:  true,
						RetryableStatusCodes:  []int{502, 503},
						RedeliveriesPerSecond: intPtr(10),
						EnforceGetHttpRequestMethodForHealthCheck: true,
					},
					Trigger: &eventv1.EventTrigger{
						ResponseFilter: &eventv1.ResponseFilter{
							Paths: []string{"$.data.name", "$.data.id"},
							Mode:  eventv1.ResponseFilterModeInclude,
						},
						SelectionFilter: &eventv1.SelectionFilter{
							Attributes: map[string]string{"type": "order.created"},
							Expression: &apiextensionsv1.JSON{Raw: []byte(`{"eq":["source","myapp"]}`)},
						},
					},
					Scopes: []string{"scope-a", "scope-b"},
				},
				Status: eventv1.EventSubscriptionStatus{
					URL: "https://gateway.example.com/events/sse/subscription-1",
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionTrue,
							Message: "subscribed",
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(data.DeliveryType).To(Equal("CALLBACK"))
			Expect(data.CallbackURL).ToNot(BeNil())
			Expect(*data.CallbackURL).To(Equal("https://consumer.example.com/events"))
			Expect(data.OwnerAppName).To(Equal("consumer-app"))
			Expect(data.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(data.TargetEventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("subscribed"))
			Expect(data.Meta.Environment).To(Equal("prod"))

			// Delivery
			Expect(data.Delivery).ToNot(BeNil())
			Expect(data.Delivery.Payload).To(Equal("Data"))
			Expect(data.Delivery.EventRetentionTime).To(Equal("7d"))
			Expect(data.Delivery.CircuitBreakerOptOut).To(BeTrue())
			Expect(data.Delivery.RetryableStatusCodes).To(Equal([]int{502, 503}))
			Expect(data.Delivery.RedeliveriesPerSecond).ToNot(BeNil())
			Expect(*data.Delivery.RedeliveriesPerSecond).To(Equal(10))
			Expect(data.Delivery.EnforceGetHttpRequestMethodForHealthCheck).To(BeTrue())

			// Trigger
			Expect(data.Trigger).ToNot(BeNil())
			Expect(data.Trigger.ResponseFilter).ToNot(BeNil())
			Expect(data.Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.name", "$.data.id"}))
			Expect(data.Trigger.ResponseFilter.Mode).To(Equal("Include"))
			Expect(data.Trigger.SelectionFilter).ToNot(BeNil())
			Expect(data.Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"type": "order.created"}))
			Expect(data.Trigger.SelectionFilter.Expression).To(Equal(`{"eq":["source","myapp"]}`))

			// Scopes
			Expect(data.Scopes).To(Equal([]string{"scope-a", "scope-b"}))

			Expect(data.GatewayConsumerSseUrl).To(Equal("https://gateway.example.com/events/sse/subscription-1"))
		})

		It("should set GatewayConsumerSseUrl to empty when status has no URL", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-url-sub",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "app"},
					},
					Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.GatewayConsumerSseUrl).To(BeEmpty())
		})

		It("should handle ServerSentEvent delivery with no callback", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sse-sub",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.sse.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "sse-app"},
					},
					Delivery: eventv1.Delivery{
						Type: eventv1.DeliveryTypeServerSentEvent,
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.DeliveryType).To(Equal("SERVER_SENT_EVENT"))
			Expect(data.CallbackURL).To(BeNil())
		})

		It("should derive UNKNOWN status when no conditions are set", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-conditions",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.nocond.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "app"},
					},
					Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Callback: "https://example.com"},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.StatusPhase).To(Equal("UNKNOWN"))
		})

		It("should handle trigger with only ResponseFilter (no SelectionFilter)", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "response-only",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.filter.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "filter-app"},
					},
					Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Callback: "https://example.com"},
					Trigger: &eventv1.EventTrigger{
						ResponseFilter: &eventv1.ResponseFilter{
							Paths: []string{"$.data.status"},
							Mode:  eventv1.ResponseFilterModeExclude,
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Trigger).ToNot(BeNil())
			Expect(data.Trigger.ResponseFilter).ToNot(BeNil())
			Expect(data.Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.status"}))
			Expect(data.Trigger.ResponseFilter.Mode).To(Equal("Exclude"))
			Expect(data.Trigger.SelectionFilter).To(BeNil())
		})

		It("should handle trigger with only SelectionFilter (no ResponseFilter)", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "selection-only",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.select.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "select-app"},
					},
					Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Callback: "https://example.com"},
					Trigger: &eventv1.EventTrigger{
						SelectionFilter: &eventv1.SelectionFilter{
							Attributes: map[string]string{"source": "orders"},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Trigger).ToNot(BeNil())
			Expect(data.Trigger.ResponseFilter).To(BeNil())
			Expect(data.Trigger.SelectionFilter).ToNot(BeNil())
			Expect(data.Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"source": "orders"}))
			Expect(data.Trigger.SelectionFilter.Expression).To(BeEmpty())
		})

		It("should leave trigger nil when spec has no trigger", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-trigger",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.plain.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "plain-app"},
					},
					Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Callback: "https://example.com"},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Trigger).To(BeNil())
			Expect(data.Scopes).To(BeNil())
		})

		It("should map DataRef payload type", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dataref-sub",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.dataref.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "ref-app"},
					},
					Delivery: eventv1.Delivery{
						Type:     eventv1.DeliveryTypeCallback,
						Payload:  eventv1.PayloadTypeDataRef,
						Callback: "https://example.com/events",
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Delivery).ToNot(BeNil())
			Expect(data.Delivery.Payload).To(Equal("DataRef"))
		})

		It("should map delivery with minimal fields", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-delivery",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.minimal.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "min-app"},
					},
					Delivery: eventv1.Delivery{
						Type:     eventv1.DeliveryTypeCallback,
						Callback: "https://example.com",
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Delivery).ToNot(BeNil())
			Expect(data.Delivery.CircuitBreakerOptOut).To(BeFalse())
			Expect(data.Delivery.RetryableStatusCodes).To(BeNil())
			Expect(data.Delivery.RedeliveriesPerSecond).To(BeNil())
			Expect(data.Delivery.EnforceGetHttpRequestMethodForHealthCheck).To(BeFalse())
			Expect(data.Delivery.EventRetentionTime).To(BeEmpty())
		})
	})

	Describe("KeyFromObject", func() {
		It("should return composite key from CR fields", func() {
			obj := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-subscription",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "my-app"},
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.eni.quickstart.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "consumer-app"},
					},
				},
			}

			key := t.KeyFromObject(obj)
			Expect(key.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(key.OwnerAppName).To(Equal("consumer-app"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("my-subscription"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should use CR fields from lastKnown when available", func() {
			req := k8stypes.NamespacedName{
				Namespace: "prod--platform--narvi",
				Name:      "my-subscription",
			}
			lastKnown := &eventv1.EventSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "prod--platform--narvi",
					Name:      "my-subscription",
				},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.eni.quickstart.v1",
					Requestor: ctypes.TypedObjectRef{
						ObjectRef: ctypes.ObjectRef{Name: "consumer-app"},
					},
				},
			}

			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(key.OwnerAppName).To(Equal("consumer-app"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
		})

		It("should fall back to convention when lastKnown is nil", func() {
			req := k8stypes.NamespacedName{
				Namespace: "prod--platform--narvi",
				Name:      "my-subscription",
			}

			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.EventType).To(Equal("my-subscription"))
			Expect(key.OwnerAppName).To(Equal("my-subscription"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
		})
	})
})

func intPtr(i int) *int {
	return &i
}
