// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
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
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
)

var _ = Describe("Listener Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		listener := &spectrev1.Listener{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Listener")
			err := k8sClient.Get(ctx, typeNamespacedName, listener)
			if err != nil && errors.IsNotFound(err) {
				resource := &spectrev1.Listener{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: spectrev1.ListenerSpec{
						Consumer: ctypes.TypedObjectRef{
							TypeMeta: metav1.TypeMeta{
								Kind:       "Application",
								APIVersion: "application.cp.ei.telekom.de/v1",
							},
							ObjectRef: ctypes.ObjectRef{
								Name:      "consumer-app",
								Namespace: resourceNamespace,
							},
						},
						Provider: ctypes.TypedObjectRef{
							TypeMeta: metav1.TypeMeta{
								Kind:       "Application",
								APIVersion: "application.cp.ei.telekom.de/v1",
							},
							ObjectRef: ctypes.ObjectRef{
								Name:      "provider-app",
								Namespace: resourceNamespace,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &spectrev1.Listener{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Listener")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &ListenerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&handler.ListenerHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
