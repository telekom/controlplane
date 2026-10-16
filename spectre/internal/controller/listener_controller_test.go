// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
						Application: ctypes.ObjectRef{
							Name:      "sa-consumer-app",
							Namespace: resourceNamespace,
						},
						// Required by the ListenerSpec XValidation rule: at least one
						// of apiListener or eventListener must be set.
						ApiListener: &spectrev1.ApiListener{
							ApiBasePath: "/api/v1/orders",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &spectrev1.Listener{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if errors.IsNotFound(err) {
				return // Already cleaned up by the controller or a previous test.
			}
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Listener")
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
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

			// The manager-registered controller may reconcile the same object
			// concurrently, causing a conflict on the status update. Retry
			// until the cache catches up and no conflict occurs.
			Eventually(func(g Gomega) {
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				g.Expect(err).NotTo(HaveOccurred())
			}, testTimeout, testInterval).Should(Succeed())
		})
	})
})

var _ = Describe("Listener Controller wiring", func() {
	It("gives the Listener handler the manager's uncached API reader", func() {
		impl, ok := managedListenerReconciler.Controller.(*cc.ControllerImpl[*spectrev1.Listener])
		Expect(ok).To(BeTrue())
		h, ok := impl.Handler.(*handler.ListenerHandler)
		Expect(ok).To(BeTrue())
		Expect(h.Reader).NotTo(BeNil())
		Expect(h.Reader).To(BeIdenticalTo(testMgr.GetAPIReader()))
	})
})

var _ = Describe("Listener validation", func() {
	ctx := context.Background()

	appRef := func(kind, name string) ctypes.TypedObjectRef {
		return ctypes.TypedObjectRef{
			TypeMeta:  metav1.TypeMeta{Kind: kind, APIVersion: "application.cp.ei.telekom.de/v1"},
			ObjectRef: ctypes.ObjectRef{Name: name, Namespace: "default"},
		}
	}

	newListener := func(name string, consumer, provider ctypes.TypedObjectRef) *spectrev1.Listener {
		return &spectrev1.Listener{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: spectrev1.ListenerSpec{
				Consumer:    consumer,
				Provider:    provider,
				Application: ctypes.ObjectRef{Name: "sa-consumer-app", Namespace: "default"},
				ApiListener: &spectrev1.ApiListener{ApiBasePath: "/api/v1/validation"},
			},
		}
	}

	expectInvalid := func(listener *spectrev1.Listener, field, message string) {
		err := k8sClient.Create(ctx, listener)
		Expect(err).To(HaveOccurred())
		Expect(errors.IsInvalid(err)).To(BeTrue())
		statusErr, ok := err.(errors.APIStatus)
		Expect(ok).To(BeTrue())
		Expect(statusErr.Status().Details.Causes).To(ContainElement(SatisfyAll(
			HaveField("Type", metav1.CauseTypeFieldValueInvalid),
			HaveField("Field", field),
			HaveField("Message", HaveSuffix(message)),
		)))
	}

	It("rejects a consumer that is not an Application", func() {
		listener := newListener("validation-consumer-team", appRef("Team", "consumer-app"), appRef("Application", "provider-app"))
		expectInvalid(listener, "spec.consumer", "consumer must reference an Application")
	})

	It("rejects a provider that is not an Application", func() {
		listener := newListener("validation-provider-team", appRef("Application", "consumer-app"), appRef("Team", "provider-app"))
		expectInvalid(listener, "spec.provider", "provider must reference an Application")
	})

	It("rejects a consumer without a kind", func() {
		listener := newListener("validation-consumer-empty", appRef("", "consumer-app"), appRef("Application", "provider-app"))
		expectInvalid(listener, "spec.consumer", "consumer must reference an Application")
	})

	It("accepts a consumer and provider that are Applications", func() {
		listener := newListener("validation-applications", appRef("Application", "consumer-app"), appRef("Application", "provider-app"))
		Expect(k8sClient.Create(ctx, listener)).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, listener))).To(Succeed())
	})
})
