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

	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	fileexposure_handler "github.com/telekom/controlplane/file/internal/handler/fileexposure"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FileExposure Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		fileexposureObj := &filev1.FileExposure{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind FileExposure")
			err := k8sClient.Get(ctx, typeNamespacedName, fileexposureObj)
			if err != nil && errors.IsNotFound(err) {
				resource := &filev1.FileExposure{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: filev1.FileExposureSpec{
						FileType:   "test-filetype",
						Visibility: filev1.VisibilityEnterprise,
						Approval:   filev1.Approval{Strategy: filev1.ApprovalStrategyAuto},
						Zone:       &ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &filev1.FileExposure{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance FileExposure")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &FileExposureReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&fileexposure_handler.FileExposureHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
