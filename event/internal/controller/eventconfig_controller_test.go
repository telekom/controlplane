// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventconfig"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventConfig Controller", func() {
	It("fans out backend status changes to its proxy and other mesh peers", func() {
		const env = "callback-cascade-test"
		labels := map[string]string{cconfig.EnvironmentLabelKey: env}
		backend := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-cascade-backend", Namespace: "default", Labels: labels},
			Spec:       eventv1.EventConfigSpec{Zone: ctypes.ObjectRef{Name: "backend", Namespace: "default"}, Local: &eventv1.LocalBackend{Admin: eventv1.AdminConfig{Url: "https://admin.example.com"}, ServerSendEventUrl: "https://sse.example.com", PublishEventUrl: "https://publish.example.com"}},
		}
		proxy := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-cascade-proxy", Namespace: "default", Labels: labels},
			Spec:       eventv1.EventConfigSpec{Zone: ctypes.ObjectRef{Name: "exposure", Namespace: "default"}, Proxy: &eventv1.ProxyBackend{TargetZone: backend.Spec.Zone}},
		}
		peer := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-cascade-peer", Namespace: "default", Labels: labels},
			Spec:       eventv1.EventConfigSpec{Zone: ctypes.ObjectRef{Name: "subscriber", Namespace: "default"}, Proxy: &eventv1.ProxyBackend{TargetZone: backend.Spec.Zone}},
		}
		for _, cfg := range []*eventv1.EventConfig{backend, proxy, peer} {
			Expect(k8sClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, cfg)).To(Succeed()) })
		}
		backend.Status.ProxyCallbackURLs = map[string]string{"exposure": "https://backend/old"}
		Expect(k8sClient.Status().Update(ctx, backend)).To(Succeed())
		backend.Status.ProxyCallbackURLs["exposure"] = "https://backend/new"
		Expect(k8sClient.Status().Update(ctx, backend)).To(Succeed())
		r := &EventConfigReconciler{Client: k8sClient}
		Expect(r.MapEventConfigToEventConfig(ctx, backend)).To(ConsistOf(
			reconcile.Request{NamespacedName: types.NamespacedName{Name: proxy.Name, Namespace: proxy.Namespace}},
			reconcile.Request{NamespacedName: types.NamespacedName{Name: peer.Name, Namespace: peer.Namespace}},
		))
	})
	It("enqueues a proxy EventConfig when its backend zone changes", func() {
		mapCtx := context.Background()
		proxyCfg := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-backend-proxy", Namespace: "default", Labels: map[string]string{
				cconfig.EnvironmentLabelKey: "callback-backend-test",
			}},
			Spec: eventv1.EventConfigSpec{
				Zone:  ctypes.ObjectRef{Name: "exposure-zone", Namespace: "default"},
				Proxy: &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "backend-zone", Namespace: "default"}},
			},
		}
		Expect(k8sClient.Create(mapCtx, proxyCfg)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(mapCtx, proxyCfg)).To(Succeed()) })
		backendZone := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "backend-zone", Namespace: "default", Labels: map[string]string{
				cconfig.EnvironmentLabelKey: "callback-backend-test",
			}},
		}
		r := &EventConfigReconciler{Client: k8sClient}
		Expect(r.MapZoneToEventConfig(mapCtx, backendZone)).To(ConsistOf(reconcile.Request{
			NamespacedName: types.NamespacedName{Name: proxyCfg.Name, Namespace: proxyCfg.Namespace},
		}))
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		reconcileCtx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		eventconfigObj := &eventv1.EventConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind EventConfig")
			err := k8sClient.Get(reconcileCtx, typeNamespacedName, eventconfigObj)
			if err != nil && errors.IsNotFound(err) {
				resource := &eventv1.EventConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: eventv1.EventConfigSpec{
						Zone: ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
						Local: &eventv1.LocalBackend{
							Admin: eventv1.AdminConfig{
								Url: "https://admin.example.com",
								Client: eventv1.ClientConfig{
									Realm: ctypes.ObjectRef{Name: "test-realm", Namespace: "default"},
								},
							},
							ServerSendEventUrl: "https://sse.example.com",
							PublishEventUrl:    "https://publish.example.com",
						},
						Mesh: &eventv1.MeshConfig{
							FullMesh: true,
							Client: eventv1.ClientConfig{
								Realm: ctypes.ObjectRef{Name: "test-realm", Namespace: "default"},
							},
						},
					},
				}
				Expect(k8sClient.Create(reconcileCtx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &eventv1.EventConfig{}
			err := k8sClient.Get(reconcileCtx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance EventConfig")
			Expect(k8sClient.Delete(reconcileCtx, resource)).To(Succeed())
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

			_, err := controllerReconciler.Reconcile(reconcileCtx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("persists, replaces, and clears the optional Horizon environment overwrite", func() {
			resource := &eventv1.EventConfig{}
			Expect(k8sClient.Get(reconcileCtx, typeNamespacedName, resource)).To(Succeed())

			resource.Spec.OverwriteEnvironmentName = "legacy-horizon"
			Expect(k8sClient.Update(reconcileCtx, resource)).To(Succeed())
			Expect(k8sClient.Get(reconcileCtx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Spec.OverwriteEnvironmentName).To(Equal("legacy-horizon"))

			resource.Spec.OverwriteEnvironmentName = "replacement"
			Expect(k8sClient.Update(reconcileCtx, resource)).To(Succeed())
			Expect(k8sClient.Get(reconcileCtx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Spec.OverwriteEnvironmentName).To(Equal("replacement"))

			resource.Spec.OverwriteEnvironmentName = ""
			Expect(k8sClient.Update(reconcileCtx, resource)).To(Succeed())
			Expect(k8sClient.Get(reconcileCtx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Spec.OverwriteEnvironmentName).To(BeEmpty())
		})
	})
})
