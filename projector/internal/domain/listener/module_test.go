// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	appv1 "github.com/telekom/controlplane/application/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("SpectreApplication watch", func() {
	It("enqueues only indexed listeners referencing the changed application", func() {
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		makeListener := func(namespace, name, app string) *spectrev1.Listener {
			return &spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
				Spec: spectrev1.ListenerSpec{Application: ctypes.ObjectRef{Name: app}}}
		}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			makeListener("ns", "matched", "target"), makeListener("ns", "unrelated", "other"),
			makeListener("other-ns", "same-name", "target"),
			&spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "other-ns", Name: "cross-namespace"},
				Spec: spectrev1.ListenerSpec{Application: ctypes.ObjectRef{Namespace: "ns", Name: "target"}}},
		).WithIndex(&spectrev1.Listener{}, applicationIndex, func(obj client.Object) []string {
			l := obj.(*spectrev1.Listener)
			ns := l.Spec.Application.Namespace
			if ns == "" {
				ns = l.Namespace
			}
			return []string{ns + "/" + l.Spec.Application.Name}
		}).Build()
		app := &spectrev1.SpectreApplication{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "target"}}
		requests := listenersForApplication(context.Background(), reader, app)
		Expect(requests).To(ConsistOf(
			HaveField("NamespacedName", client.ObjectKey{Namespace: "ns", Name: "matched"}),
			HaveField("NamespacedName", client.ObjectKey{Namespace: "other-ns", Name: "cross-namespace"}),
		))
	})

})

var _ = Describe("Parent watches", func() {
	It("matches subscription and exposure by owner team, name and exact base path", func() {
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		makeListener := func(name, consumer, provider, path string) *spectrev1.Listener {
			return &spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "listeners", Name: name}, Spec: spectrev1.ListenerSpec{
				Consumer:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--consumer-team", Name: consumer}},
				Provider:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--provider-team", Name: provider}},
				ApiListener: &spectrev1.ApiListener{ApiBasePath: path},
			}}
		}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			makeListener("matched", "consumer", "provider", "/api"),
			makeListener("different-path", "consumer", "provider", "/api/v2"),
			makeListener("different-consumer", "other", "provider", "/api"),
			makeListener("different-provider", "consumer", "other", "/api"),
			&spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "listeners", Name: "different-team"}, Spec: spectrev1.ListenerSpec{
				Consumer:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--other-team", Name: "consumer"}},
				Provider:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--other-team", Name: "provider"}},
				ApiListener: &spectrev1.ApiListener{ApiBasePath: "/api"},
			}},
		).WithIndex(&spectrev1.Listener{}, consumerIndex, indexConsumer).
			WithIndex(&spectrev1.Listener{}, providerIndex, indexProvider).
			WithIndex(&spectrev1.Listener{}, subscriptionIndex, indexSubscription).
			WithIndex(&spectrev1.Listener{}, exposureIndex, indexExposure).Build()
		ctx := context.Background()
		sub := &apiv1.ApiSubscription{ObjectMeta: metav1.ObjectMeta{Namespace: "prod--consumer-team", Name: "unrelated-cr-name"},
			Spec: apiv1.ApiSubscriptionSpec{ApiBasePath: "/api", Requestor: apiv1.Requestor{Application: ctypes.ObjectRef{Name: "consumer"}}}}
		Expect(listenersForParent(ctx, reader, sub)).To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "matched"}},
			reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "different-provider"}}))
		exposure := &apiv1.ApiExposure{ObjectMeta: metav1.ObjectMeta{Namespace: "prod--provider-team", Labels: map[string]string{"cp.ei.telekom.de/application": "provider"}},
			Spec: apiv1.ApiExposureSpec{ApiBasePath: "/api"}}
		Expect(listenersForParent(ctx, reader, exposure)).To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "matched"}},
			reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "different-consumer"}}))
		sub.Spec.ApiBasePath = "/api/old"
		Expect(listenersForParent(ctx, reader, sub)).To(BeEmpty())
	})

	It("finds observer and direct owner listeners on Application events", func() {
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&spectrev1.SpectreApplication{ObjectMeta: metav1.ObjectMeta{Namespace: "spectre", Name: "observer-ref"},
				Spec: spectrev1.SpectreApplicationSpec{Application: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--team", Name: "owner"}}}},
			&spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "listeners", Name: "observer"},
				Spec: spectrev1.ListenerSpec{Application: ctypes.ObjectRef{Namespace: "spectre", Name: "observer-ref"}}},
			&spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "listeners", Name: "consumer"},
				Spec: spectrev1.ListenerSpec{Consumer: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--team", Name: "owner"}}}},
			&spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "listeners", Name: "provider"},
				Spec: spectrev1.ListenerSpec{Provider: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--team", Name: "owner"}}}},
		).WithIndex(&spectrev1.SpectreApplication{}, observerIndex, indexObserver).
			WithIndex(&spectrev1.Listener{}, applicationIndex, func(obj client.Object) []string {
				ref := obj.(*spectrev1.Listener).Spec.Application
				if ref.Name == "" {
					return nil
				}
				if ref.Namespace == "" {
					ref.Namespace = obj.GetNamespace()
				}
				return []string{ref.Namespace + "/" + ref.Name}
			}).WithIndex(&spectrev1.Listener{}, consumerIndex, indexConsumer).
			WithIndex(&spectrev1.Listener{}, providerIndex, indexProvider).Build()
		app := &appv1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "prod--team", Name: "owner"}}
		app.Spec.Team = "team"
		Expect(listenersForParent(context.Background(), reader, app)).To(ConsistOf(
			reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "observer"}},
			reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "consumer"}},
			reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "provider"}},
		))
	})

	It("enqueues both old and new subscription keys on update, and the deleted key", func() {
		scheme := runtime.NewScheme()
		Expect(spectrev1.AddToScheme(scheme)).To(Succeed())
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "listeners", Name: "old"}, Spec: spectrev1.ListenerSpec{
				Consumer: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--team", Name: "consumer"}}, ApiListener: &spectrev1.ApiListener{ApiBasePath: "/old"}}},
			&spectrev1.Listener{ObjectMeta: metav1.ObjectMeta{Namespace: "listeners", Name: "new"}, Spec: spectrev1.ListenerSpec{
				Consumer: ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Namespace: "prod--team", Name: "consumer"}}, ApiListener: &spectrev1.ApiListener{ApiBasePath: "/new"}}},
		).WithIndex(&spectrev1.Listener{}, subscriptionIndex, indexSubscription).Build()
		old := &apiv1.ApiSubscription{ObjectMeta: metav1.ObjectMeta{Namespace: "prod--team"}, Spec: apiv1.ApiSubscriptionSpec{
			ApiBasePath: "/old", Requestor: apiv1.Requestor{Application: ctypes.ObjectRef{Name: "consumer"}}}}
		newObj := old.DeepCopy()
		newObj.Spec.ApiBasePath = "/new"
		q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		DeferCleanup(q.ShutDown)
		h := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return listenersForParent(ctx, reader, obj)
		})
		h.Update(context.Background(), event.UpdateEvent{ObjectOld: old, ObjectNew: newObj}, q)
		Expect(q.Len()).To(Equal(2))
		h.Delete(context.Background(), event.DeleteEvent{Object: old}, q)
		Expect(q.Len()).To(Equal(2))
		first, _ := q.Get()
		second, _ := q.Get()
		Expect([]reconcile.Request{first, second}).To(ConsistOf(
			reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "old"}},
			reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "listeners", Name: "new"}},
		))
		q.Done(first)
		q.Done(second)
	})
})
