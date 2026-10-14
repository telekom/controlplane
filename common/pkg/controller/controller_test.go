// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/testutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	common_types "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("recordStatusUpdate", func() {
	It("uses unknown when the object kind cannot be resolved", func() {
		before := testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("skipped", "unknown"))

		recordStatusUpdate(statusUpdateResultSkipped, &test.TestResource{}, runtime.NewScheme())

		Expect(testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("skipped", "unknown"))).To(Equal(before + 1))
	})
})

var _ = Describe("EnsureNotReadyOnError", func() {
	DescribeTable("sets the Ready condition",
		func(existing *metav1.Condition, err error, expected metav1.Condition) {
			obj := test.NewObject("fake", "default")
			if existing != nil {
				obj.SetCondition(*existing)
			}

			EnsureNotReadyOnError(ctx, nil, obj, err)

			ready := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(expected.Status))
			Expect(ready.Reason).To(Equal(expected.Reason))
			Expect(ready.Message).To(Equal(expected.Message))
		},
		Entry("uses reason Error for a generic error",
			nil, errors.New("boom"),
			condition.NewNotReadyCondition(condition.ReasonError, "boom")),
		Entry("replaces Ready=True for a generic error",
			ptr.To(condition.NewReadyCondition(condition.ReasonProvisioned, "ready")), errors.New("boom"),
			condition.NewNotReadyCondition(condition.ReasonError, "boom")),
		Entry("replaces Ready=Unknown for a generic error",
			ptr.To(condition.SetToUnknown(condition.ReadyCondition)), errors.New("boom"),
			condition.NewNotReadyCondition(condition.ReasonError, "boom")),
		Entry("does not rewrite Ready=False/Error for a second generic error with a different message",
			ptr.To(condition.NewNotReadyCondition(condition.ReasonError, "dial tcp 10.0.0.1:41234: connection refused")),
			errors.New("dial tcp 10.0.0.1:52817: connection refused"),
			condition.NewNotReadyCondition(condition.ReasonError, "dial tcp 10.0.0.1:41234: connection refused")),
		Entry("replaces a Ready=False/Blocked left by an earlier blocked error for a generic error",
			ptr.To(condition.NewNotReadyCondition(condition.ReasonBlocked, "route missing")), errors.New("boom"),
			condition.NewNotReadyCondition(condition.ReasonError, "boom")),
		Entry("keeps a Ready=False with a handler-specific reason for a generic error",
			ptr.To(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "visibility not allowed")), errors.New("boom"),
			condition.NewNotReadyCondition(condition.ReasonAccessDenied, "visibility not allowed")),
		Entry("ignores a conflict error",
			ptr.To(condition.NewReadyCondition(condition.ReasonProvisioned, "ready")),
			apierrors.NewConflict(schema.GroupResource{Resource: "testresources"}, "fake", errors.New("modified")),
			condition.NewReadyCondition(condition.ReasonProvisioned, "ready")),
		Entry("uses reason Blocked for a blocked error",
			nil, ctrlerrors.BlockedErrorf("route missing"),
			condition.NewNotReadyCondition(condition.ReasonBlocked, "route missing")),
		Entry("uses reason Blocked for a blocked error wrapped with fmt.Errorf",
			nil, fmt.Errorf("reconciling: %w", ctrlerrors.BlockedErrorf("route missing")),
			condition.NewNotReadyCondition(condition.ReasonBlocked, "reconciling: route missing")),
		Entry("uses reason Blocked for a blocked error wrapped with pkg/errors",
			nil, pkgerrors.Wrap(ctrlerrors.BlockedErrorf("route missing"), "reconciling"),
			condition.NewNotReadyCondition(condition.ReasonBlocked, "reconciling: route missing")),
		Entry("replaces Ready=Unknown for a blocked error",
			ptr.To(condition.SetToUnknown(condition.ReadyCondition)), ctrlerrors.BlockedErrorf("route missing"),
			condition.NewNotReadyCondition(condition.ReasonBlocked, "route missing")),
		Entry("replaces a Ready=False/Error left by an earlier error for a blocked error",
			ptr.To(condition.NewNotReadyCondition(condition.ReasonError, "transient")), ctrlerrors.BlockedErrorf("route missing"),
			condition.NewNotReadyCondition(condition.ReasonBlocked, "route missing")),
		Entry("replaces a Ready=False/Blocked left by an earlier blocked error for a blocked error",
			ptr.To(condition.NewNotReadyCondition(condition.ReasonBlocked, "application id not resolved")), ctrlerrors.BlockedErrorf("route missing"),
			condition.NewNotReadyCondition(condition.ReasonBlocked, "route missing")),
		Entry("keeps a Ready=False with a handler-specific reason for a blocked error",
			ptr.To(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "visibility not allowed")), ctrlerrors.BlockedErrorf("blocked"),
			condition.NewNotReadyCondition(condition.ReasonAccessDenied, "visibility not allowed")),
	)
})

var _ = Describe("StampObservedGeneration", func() {
	It("should set ObservedGeneration on all conditions to the object's generation", func() {
		obj := test.NewObject("test-stamp", "default")
		obj.SetGeneration(5)
		obj.SetCondition(metav1.Condition{
			Type:   condition.ConditionTypeProcessing,
			Status: metav1.ConditionTrue,
			Reason: "Testing",
		})
		obj.SetCondition(metav1.Condition{
			Type:   condition.ConditionTypeReady,
			Status: metav1.ConditionFalse,
			Reason: "NotReady",
		})

		Expect(obj.GetConditions()).To(HaveLen(2))
		for _, c := range obj.GetConditions() {
			Expect(c.ObservedGeneration).To(Equal(int64(0)))
		}

		StampObservedGeneration(obj)

		for _, c := range obj.GetConditions() {
			Expect(c.ObservedGeneration).To(Equal(int64(5)))
		}
	})

	It("should update existing ObservedGeneration to current generation", func() {
		obj := test.NewObject("test-stamp-update", "default")
		obj.SetGeneration(3)
		obj.SetCondition(metav1.Condition{
			Type:               condition.ConditionTypeProcessing,
			Status:             metav1.ConditionFalse,
			Reason:             "Done",
			ObservedGeneration: 1,
		})

		StampObservedGeneration(obj)

		cond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
		Expect(cond.ObservedGeneration).To(Equal(int64(3)))
	})

	It("should be a no-op when there are no conditions", func() {
		obj := test.NewObject("test-stamp-empty", "default")
		obj.SetGeneration(7)

		Expect(obj.GetConditions()).To(BeEmpty())

		StampObservedGeneration(obj)

		Expect(obj.GetConditions()).To(BeEmpty())
	})
})

var _ = Describe("Controller", func() {
	templ := &test.TestResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: environment,
			},
		},
	}

	Context("NewController", func() {
		DescribeTable("derives the field owner from the resource type",
			func(fieldOwner func() string, expected string) {
				Expect(fieldOwner()).To(Equal(expected))
			},
			Entry("single-word interface", func() string { return fieldOwnerFor[common_types.Object]() }, "object-controller"),
			Entry("multi-word pointer", func() string { return fieldOwnerFor[*test.TestResource]() }, "testresource-controller"),
		)

		It("uses the resource field owner for writes", func() {
			var fieldManager string
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).ToNot(HaveOccurred())
			interceptedClient := interceptor.NewClient(watchClient, interceptor.Funcs{
				Update: func(_ context.Context, _ client.WithWatch, _ client.Object, opts ...client.UpdateOption) error {
					options := &client.UpdateOptions{}
					for _, opt := range opts {
						opt.ApplyToUpdate(options)
					}
					fieldManager = options.FieldManager
					return nil
				},
			})
			controller := NewController(handler.NewNopHandler[*test.TestResource](), interceptedClient, &mock.EventRecorder{}).(*ControllerImpl[*test.TestResource])

			Expect(controller.Client.Update(ctx, &test.TestResource{})).To(Succeed())
			Expect(fieldManager).To(Equal("testresource-controller"))
		})

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
			Expect(res).To(Equal(reconcile.Result{Requeue: true}))

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
			Expect(obj.GetConditions()).To(HaveLen(1))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady).Status).To(Equal(metav1.ConditionUnknown))
		})

		It("should not update status when it is unchanged", func() {
			var statusUpdates atomic.Int32
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).ToNot(HaveOccurred())
			interceptedClient := interceptor.NewClient(watchClient, interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					if subResourceName == "status" {
						statusUpdates.Add(1)
					}
					return c.SubResource(subResourceName).Update(ctx, obj, opts...)
				},
			})
			controller := NewController(nopHandler, interceptedClient, &recorder)
			obj := templ.DeepCopy()
			obj.SetName("unchanged-status")
			obj.SetNamespace(req.Namespace)
			unchangedRequest := reconcile.Request{NamespacedName: client.ObjectKey{
				Name:      obj.Name,
				Namespace: obj.Namespace,
			}}

			Expect(interceptedClient.Create(ctx, obj)).To(Succeed())
			_, err = controller.Reconcile(ctx, unchangedRequest, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			_, err = controller.Reconcile(ctx, unchangedRequest, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			_, err = controller.Reconcile(ctx, unchangedRequest, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())

			Expect(interceptedClient.Get(ctx, unchangedRequest.NamespacedName, obj)).To(Succeed())
			statusUpdates.Store(0)
			skippedBefore := testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("skipped", "testresource"))

			_, err = controller.Reconcile(ctx, unchangedRequest, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			Expect(statusUpdates.Load()).To(BeZero())
			Expect(testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("skipped", "testresource"))).To(Equal(skippedBefore + 1))
		})

		It("should still update status when only the generation changed", func() {
			controller := NewController(nopHandler, k8sClient, &recorder)
			obj := templ.DeepCopy()
			obj.SetName("generation-bump")
			obj.SetNamespace(req.Namespace)
			bumpRequest := reconcile.Request{NamespacedName: client.ObjectKey{
				Name:      obj.Name,
				Namespace: obj.Namespace,
			}}

			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			for range 3 {
				_, err := controller.Reconcile(ctx, bumpRequest, &test.TestResource{})
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, bumpRequest.NamespacedName, obj)).To(Succeed())
			obj.Spec.Properties = &runtime.RawExtension{Raw: []byte(`{"changed":true}`)}
			Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			updatedBefore := testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("updated", "testresource"))

			_, err := controller.Reconcile(ctx, bumpRequest, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			Expect(testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("updated", "testresource"))).To(Equal(updatedBefore + 1))

			Expect(k8sClient.Get(ctx, bumpRequest.NamespacedName, obj)).To(Succeed())
			for _, c := range obj.GetConditions() {
				Expect(c.ObservedGeneration).To(Equal(obj.GetGeneration()),
					"conditions must not report a stale ObservedGeneration")
			}
		})

		It("counts failed status updates as errors", func() {
			statusErr := errors.New("status update failed")
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).ToNot(HaveOccurred())
			failingClient := interceptor.NewClient(watchClient, interceptor.Funcs{
				SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
					return statusErr
				},
			})
			controller := NewController(nopHandler, failingClient, &recorder).(*ControllerImpl[*test.TestResource])
			original := &test.TestResource{}
			Expect(k8sClient.Get(ctx, req.NamespacedName, original)).To(Succeed())
			changed := original.DeepCopy()
			changed.SetCondition(condition.NewBlockedCondition("changed"))
			updatedBefore := testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("updated", "testresource"))
			skippedBefore := testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("skipped", "testresource"))
			errorBefore := testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("error", "testresource"))

			err = controller.updateStatusIfChanged(ctx, original, changed)

			Expect(err).To(MatchError(statusErr))
			Expect(testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("updated", "testresource"))).To(Equal(updatedBefore))
			Expect(testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("skipped", "testresource"))).To(Equal(skippedBefore))
			Expect(testutil.ToFloat64(statusUpdatesTotal.WithLabelValues("error", "testresource"))).To(Equal(errorBefore + 1))
		})

		It("should handle generic errors", func() {
			controller := NewController(errorHandler, k8sClient, &recorder)

			res, err := controller.Reconcile(ctx, req, &test.TestResource{})
			Expect(err).To(MatchError("test error"))
			Expect(res).To(Equal(reconcile.Result{}))

			var obj test.TestResource
			Expect(k8sClient.Get(ctx, req.NamespacedName, &obj)).To(Succeed())
			Expect(obj.GetConditions()).To(HaveLen(1))
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady).Status).To(Equal(metav1.ConditionFalse))
		})

		It("should use custom RequeueAfter when handler sets it via ReconcileHint", func() {
			customRequeue := 10 * time.Second
			customHandler := handler.NewCustomHandler(
				func(ctx context.Context, object *test.TestResource) error {
					contextutil.SetRequeueAfter(ctx, customRequeue)
					return nil
				},
				func(ctx context.Context, obj *test.TestResource) error {
					return nil
				},
			)
			controller := NewController(customHandler, k8sClient, &recorder)

			res, err := controller.Reconcile(ctx, req, &test.TestResource{})
			Expect(err).ToNot(HaveOccurred())
			// The hint value is used directly (no jitter) so the result should equal the hint.
			Expect(res.RequeueAfter).To(Equal(customRequeue))
		})
	})

	Context("Reconciler", func() {
		timeout := 2 * time.Second
		interval := 200 * time.Millisecond

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
