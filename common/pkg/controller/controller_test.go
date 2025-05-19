// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	opErrors "github.com/telekom/controlplane/common/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Controller", func() {

	var (
		templ = &test.TestResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: environment,
				},
			},
		}
	)

	Context("NewController", func() {
		It("should return a new ControllerImpl", func() {
			controller := NewController(handler.NewNopHandler[*test.TestResource](), k8sClient, &mock.EventRecorder{})
			Expect(controller).To(BeAssignableToTypeOf(&ControllerImpl[*test.TestResource]{}))
		})
	})

	Context("Controller", func() {
		var (
			recorder = mock.EventRecorder{}
			req      = reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      name,
					Namespace: "no-manager",
				},
			}
			errorHandler = handler.NewCustomHandler(
				func(ctx context.Context, object *test.TestResource) error {
					return fmt.Errorf("test error")
				},
				func(ctx context.Context, obj *test.TestResource) error {
					return fmt.Errorf("test error")
				},
			)
			operatorErrorHandler = handler.NewCustomHandler(
				func(ctx context.Context, object *test.TestResource) error {
					return opErrors.NewRetriableResourcesError(fmt.Errorf("test error"), "operator error message")
				},
				func(ctx context.Context, obj *test.TestResource) error {
					return opErrors.NewRetriableResourcesError(fmt.Errorf("test error"), "operator error message")
				},
			)
			nopHandler = handler.NewNopHandler[*test.TestResource]()
		)

		It("should return when the resource does not exist", func() {
			controller := NewController(nopHandler, k8sClient, &recorder)

			res, err := controller.Reconcile(ctx, req, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))
		})

		It("should handle the first-setup", func() {
			controller := NewController(nopHandler, k8sClient, &recorder)

			obj := templ.DeepCopy()
			obj.SetNamespace(req.Namespace)

			Expect(k8sClient.Create(ctx, obj)).To(Succeed())

			res, err := controller.Reconcile(ctx, req, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			Expect(k8sClient.Get(ctx, req.NamespacedName, obj)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(obj, config.FinalizerName)).To(BeTrue())
		})

		It("should set the correct conditions when there is no error", func() {
			controller := NewController(nopHandler, k8sClient, &recorder)

			res, err := controller.Reconcile(ctx, req, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))

			var obj test.TestResource
			Expect(k8sClient.Get(ctx, req.NamespacedName, &obj)).To(Succeed())
			Expect(obj.GetConditions()).To(HaveLen(2))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing).Status).To(Equal(metav1.ConditionUnknown))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady).Status).To(Equal(metav1.ConditionUnknown))
		})

		It("should handle generic errors", func() {
			controller := NewController(errorHandler, k8sClient, &recorder)

			res, err := controller.Reconcile(ctx, req, &test.TestResource{})
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			var obj test.TestResource
			Expect(k8sClient.Get(ctx, req.NamespacedName, &obj)).To(Succeed())
			Expect(obj.GetConditions()).To(HaveLen(2))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing).Status).To(Equal(metav1.ConditionUnknown))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady).Status).To(Equal(metav1.ConditionFalse))
		})

		It("should handle operator specific errors", func() {
			controller := NewController(operatorErrorHandler, k8sClient, &recorder)

			res, err := controller.Reconcile(ctx, req, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Requeue).To(BeTrue())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))

			var obj test.TestResource
			Expect(k8sClient.Get(ctx, req.NamespacedName, &obj)).To(Succeed())
			Expect(obj.GetConditions()).To(HaveLen(2))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing).Status).To(Equal(metav1.ConditionUnknown))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady).Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("Reconciler", func() {

		var timeout = 2 * time.Second
		var interval = 200 * time.Millisecond

		AfterEach(func() {
			obj := templ.DeepCopy()
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: namespace,
				}, obj)

				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

			}, timeout, interval).Should(Succeed())
		})

		It("should add a finalizer", func() {
			obj := templ.DeepCopy()
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: namespace,
				}, obj)

				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(obj.GetFinalizers()).To(ContainElement(config.FinalizerName))

			}, timeout, interval).Should(Succeed())

		})

		It("should fail with missing environment", func() {
			obj := templ.DeepCopy()
			obj.SetLabels(nil)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: namespace,
				}, obj)

				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(obj.GetConditions()).To(HaveLen(2))
				condition := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
				g.Expect(condition.Type).To(Equal("Processing"))
				g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(condition.Reason).To(Equal("Blocked"))
				g.Expect(condition.Message).To(Equal("Environment label is missing"))

			}, timeout, interval).Should(Succeed())

			obj.SetLabels(map[string]string{
				config.EnvironmentLabelKey: environment,
			})
			Expect(k8sClient.Update(ctx, obj)).To(Succeed())

		})

		It("should successfully process", func() {
			obj := templ.DeepCopy()
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: namespace,
				}, obj)

				g.Expect(err).ToNot(HaveOccurred())

			}, timeout, interval).Should(Succeed())

		})

	})
})
