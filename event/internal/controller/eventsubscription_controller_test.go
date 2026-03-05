// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventsubscription"
)

var _ = Describe("EventSubscription Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		eventsubscriptionObj := &eventv1.EventSubscription{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind EventSubscription")
			err := k8sClient.Get(ctx, typeNamespacedName, eventsubscriptionObj)
			if err != nil && errors.IsNotFound(err) {
				resource := &eventv1.EventSubscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: eventv1.EventSubscriptionSpec{
						EventType: "de.telekom.test.v1",
						Zone:      ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
						Requestor: ctypes.TypedObjectRef{
							TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
							ObjectRef: ctypes.ObjectRef{Name: "test-app", Namespace: "default"},
						},
						Delivery: eventv1.Delivery{
							Type:     eventv1.DeliveryTypeCallback,
							Payload:  eventv1.PayloadTypeData,
							Callback: "https://callback.example.com",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &eventv1.EventSubscription{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance EventSubscription")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &EventSubscriptionReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&eventsubscription.EventSubscriptionHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
