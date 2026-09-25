// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Event identifier CRD admission", func() {
	valid := []string{"orders.v1", "orders-created.v1", "de.telekom.orders-created.v1", "a-b.c-d.v1"}
	invalid := []string{"-orders", ".orders", "orders-", "orders.", "orders..created", "orders--created", "orders.-created", "orders-.created", "Orders", "orders_created"}

	for i, identifier := range valid {
		It(fmt.Sprintf("accepts EventSpecification type %q", identifier), func() {
			obj := &v1.EventSpecification{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("valid-eventspec-%d", i), Namespace: "default"},
				Spec:       v1.EventSpecificationSpec{Type: identifier, Version: "1.0.0"},
			}
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, obj)).To(Succeed()) })
		})
	}

	for i, identifier := range invalid {
		It(fmt.Sprintf("rejects EventSpecification type %q", identifier), func() {
			obj := &v1.EventSpecification{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("invalid-eventspec-%d", i), Namespace: "default"},
				Spec:       v1.EventSpecificationSpec{Type: identifier, Version: "1.0.0"},
			}
			err := k8sClient.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), fmt.Sprintf("error: %v", err))
			Expect(err.Error()).To(ContainSubstring("spec.type"))
		})
	}

	for _, reference := range []struct {
		name    string
		field   string
		makeObj func(string, string) *v1.Rover
	}{
		{
			name:  "exposure",
			field: "spec.exposures[0].event.eventType",
			makeObj: func(name, identifier string) *v1.Rover {
				return &v1.Rover{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.RoverSpec{Zone: "test-zone", ClientSecret: "topsecret", Exposures: []v1.Exposure{
						{Event: &v1.EventExposure{EventType: identifier, Visibility: v1.VisibilityEnterprise, Approval: v1.Approval{Strategy: v1.ApprovalStrategyAuto}}},
					}},
				}
			},
		},
		{
			name:  "subscription",
			field: "spec.subscriptions[0].event.eventType",
			makeObj: func(name, identifier string) *v1.Rover {
				return &v1.Rover{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.RoverSpec{Zone: "test-zone", ClientSecret: "topsecret", Subscriptions: []v1.Subscription{
						{Event: &v1.EventSubscription{
							EventType: identifier,
							Delivery:  v1.EventDelivery{Type: v1.EventDeliveryTypeServerSentEvent, Payload: v1.EventPayloadTypeData},
						}},
					}},
				}
			},
		},
	} {
		It("accepts mixed separators in Rover "+reference.name, func() {
			obj := reference.makeObj("valid-rover-"+reference.name, "de.telekom.orders-created.v1")
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, obj)).To(Succeed()) })
		})
		for i, identifier := range invalid {
			It(fmt.Sprintf("rejects Rover %s %q", reference.name, identifier), func() {
				obj := reference.makeObj(fmt.Sprintf("invalid-rover-%s-%d", reference.name, i), identifier)
				err := k8sClient.Create(ctx, obj)
				Expect(apierrors.IsInvalid(err)).To(BeTrue(), fmt.Sprintf("error: %v", err))
				Expect(err.Error()).To(ContainSubstring(reference.field))
			})
		}
	}
})
