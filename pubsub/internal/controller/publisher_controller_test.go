// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	publisherhandler "github.com/telekom/controlplane/pubsub/internal/handler/publisher"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Publisher Controller", func() {
	Context("MapSubscriberToPublisher", func() {
		It("should enqueue exactly the referenced Publisher", func() {
			reconciler := &PublisherReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			subscriber := &pubsubv1.Subscriber{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sub-1",
					Namespace: "team-a",
				},
				Spec: pubsubv1.SubscriberSpec{
					Publisher: ctypes.ObjectRef{Name: "my-publisher", Namespace: "team-b"},
				},
			}

			requests := reconciler.MapSubscriberToPublisher(ctx, subscriber)

			Expect(requests).To(HaveLen(1))
			Expect(requests[0].NamespacedName.Name).To(Equal("my-publisher"))
			Expect(requests[0].NamespacedName.Namespace).To(Equal("team-b"))
		})

		It("should return nil for non-Subscriber objects", func() {
			reconciler := &PublisherReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			publisher := &pubsubv1.Publisher{
				ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
			}

			requests := reconciler.MapSubscriberToPublisher(ctx, publisher)

			Expect(requests).To(BeNil())
		})
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		publisher := &pubsubv1.Publisher{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Publisher")
			err := k8sClient.Get(ctx, typeNamespacedName, publisher)
			if err != nil && errors.IsNotFound(err) {
				resource := &pubsubv1.Publisher{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: pubsubv1.PublisherSpec{
						EventStore:  ctypes.ObjectRef{Name: "test-store", Namespace: "default"},
						EventType:   "de.telekom.test.v1",
						PublisherId: "test-publisher-id",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &pubsubv1.Publisher{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Publisher")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &PublisherReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&publisherhandler.PublisherHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
