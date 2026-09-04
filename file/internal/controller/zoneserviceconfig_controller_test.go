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
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	zoneserviceconfig_handler "github.com/telekom/controlplane/file/internal/handler/zoneserviceconfig"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ZoneServiceConfig Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		zoneserviceconfigObj := &filev1.ZoneServiceConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ZoneServiceConfig")
			err := k8sClient.Get(ctx, typeNamespacedName, zoneserviceconfigObj)
			if err != nil && errors.IsNotFound(err) {
				resource := &filev1.ZoneServiceConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: filev1.ZoneServiceConfigSpec{
						Zone: &ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
						API: adminv1.ManagedRouteConfig{
							Name: "test-api",
							Path: "/test",
							Url:  "https://sftp.example.com",
							Type: adminv1.ManagedRouteTypeProxy,
						},
						Service: &filev1.ServiceEndpoint{
							Host: "sftp.internal",
							Port: 22,
						},
						ServiceExternal: &filev1.ServiceEndpoint{
							Host: "sftp.external",
							Port: 2222,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &filev1.ZoneServiceConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ZoneServiceConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &ZoneServiceConfigReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&zoneserviceconfig_handler.ZoneServiceConfigHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
