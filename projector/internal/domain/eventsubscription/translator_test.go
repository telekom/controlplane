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
						Type:     eventv1.DeliveryTypeCallback,
						Callback: "https://consumer.example.com/events",
					},
				},
				Status: eventv1.EventSubscriptionStatus{
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
