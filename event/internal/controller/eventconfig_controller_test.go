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
	"github.com/telekom/controlplane/event/internal/handler/eventconfig"
)

var _ = Describe("EventConfig Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		eventconfigObj := &eventv1.EventConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind EventConfig")
			err := k8sClient.Get(ctx, typeNamespacedName, eventconfigObj)
			if err != nil && errors.IsNotFound(err) {
				resource := &eventv1.EventConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: eventv1.EventConfigSpec{
						Zone: ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
						Admin: eventv1.AdminConfig{
							Url:   "https://admin.example.com",
							Realm: ctypes.ObjectRef{Name: "test-realm", Namespace: "default"},
						},
						ServerSendEventUrl: "https://sse.example.com",
						PublishEventUrl:    "https://publish.example.com",
						Mesh:               eventv1.MeshConfig{FullMesh: true},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &eventv1.EventConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance EventConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &EventConfigReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&eventconfig.EventConfigHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
