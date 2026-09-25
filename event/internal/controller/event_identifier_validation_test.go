// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Event identifier CRD admission", func() {
	valid := []string{"orders.v1", "orders-created.v1", "de.telekom.orders-created.v1", "a-b.c-d.v1"}
	invalid := []string{"-orders", ".orders", "orders-", "orders.", "orders..created", "orders--created", "orders.-created", "orders-.created", "Orders", "orders_created"}

	for _, resource := range []struct {
		name    string
		field   string
		makeObj func(string, string) client.Object
	}{
		{
			name:  "EventType",
			field: "spec.type",
			makeObj: func(name, identifier string) client.Object {
				return &eventv1.EventType{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
					Spec:       eventv1.EventTypeSpec{Type: identifier, Version: "1.0.0"},
				}
			},
		},
		{
			name:  "EventExposure",
			field: "spec.eventType",
			makeObj: func(name, identifier string) client.Object {
				return &eventv1.EventExposure{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
					Spec: eventv1.EventExposureSpec{
						EventType:  identifier,
						Visibility: eventv1.VisibilityEnterprise,
						Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
						Zone:       ctypes.ObjectRef{Name: "zone", Namespace: "default"},
						Provider:   ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "app", Namespace: "default"}},
					},
				}
			},
		},
		{
			name:  "EventSubscription",
			field: "spec.eventType",
			makeObj: func(name, identifier string) client.Object {
				return &eventv1.EventSubscription{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
					Spec: eventv1.EventSubscriptionSpec{
						EventType: identifier,
						Zone:      ctypes.ObjectRef{Name: "zone", Namespace: "default"},
						Requestor: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "app", Namespace: "default"}},
						Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent, Payload: eventv1.PayloadTypeData},
					},
				}
			},
		},
	} {
		resourceName := strings.ToLower(resource.name)
		for i, identifier := range valid {
			It(fmt.Sprintf("accepts %s identifier %q", resource.name, identifier), func() {
				obj := resource.makeObj(fmt.Sprintf("valid-%s-%d", resourceName, i), identifier)
				Expect(k8sClient.Create(ctx, obj)).To(Succeed())
				DeferCleanup(func() { Expect(k8sClient.Delete(ctx, obj)).To(Succeed()) })
			})
		}
		for i, identifier := range invalid {
			It(fmt.Sprintf("rejects %s identifier %q", resource.name, identifier), func() {
				obj := resource.makeObj(fmt.Sprintf("invalid-%s-%d", resourceName, i), identifier)
				err := k8sClient.Create(ctx, obj)
				Expect(apierrors.IsInvalid(err)).To(BeTrue(), fmt.Sprintf("error: %v", err))
				Expect(err.Error()).To(ContainSubstring(resource.field))
			})
		}
	}

	It("preserves the EventType major version suffix rule", func() {
		obj := &eventv1.EventType{
			ObjectMeta: metav1.ObjectMeta{Name: "wrong-major-suffix", Namespace: "default"},
			Spec:       eventv1.EventTypeSpec{Type: "de.telekom.orders-created.v2", Version: "1.0.0"},
		}
		err := k8sClient.Create(ctx, obj)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), fmt.Sprintf("error: %v", err))
		Expect(err.Error()).To(ContainSubstring("spec"))
	})
})
