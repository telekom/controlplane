// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventsubscription"
	"github.com/telekom/controlplane/event/internal/index"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type exposureListFailureClient struct{ client.Client }

func (c exposureListFailureClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*eventv1.EventExposureList); ok {
		return fmt.Errorf("exposure list unavailable")
	}
	return c.Client.List(ctx, list, opts...)
}

type indexedMapperClient struct {
	client.Client
	reader cache.Cache
}

func (c indexedMapperClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	options := &client.ListOptions{}
	options.ApplyOptions(opts)
	ExpectWithOffset(1, options.FieldSelector).NotTo(BeNil(), "mapper must use a cache field index")
	ExpectWithOffset(1, options.FieldSelector.Empty()).To(BeFalse(), "mapper must not scan all objects")
	ExpectWithOffset(1, options.LabelSelector).NotTo(BeNil(), "mapper must scope queries to the environment")
	return c.reader.List(ctx, list, opts...)
}

func newIndexedMapperClient() indexedMapperClient {
	cacheCtx, stop := context.WithCancel(ctx)
	reader, err := cache.New(cfg, cache.Options{Scheme: k8sClient.Scheme(), DefaultNamespaces: map[string]cache.Config{"default": {}}})
	Expect(err).To(Succeed())
	Expect(index.RegisterEventSubscriptionMapperIndices(cacheCtx, reader)).To(Succeed())
	startErrors := make(chan error, 1)
	go func() { startErrors <- reader.Start(cacheCtx) }()
	DeferCleanup(func() {
		stop()
		Eventually(startErrors, 10*time.Second).Should(Receive(Succeed()))
	})
	syncCtx, cancelSync := context.WithTimeout(cacheCtx, 10*time.Second)
	defer cancelSync()
	Expect(reader.WaitForCacheSync(syncCtx)).To(BeTrue())
	return indexedMapperClient{Client: k8sClient, reader: reader}
}

var _ = Describe("EventSubscription Controller", func() {
	It("enqueues remote callback subscriptions when the exposure EventConfig changes", func() {
		indexedClient := newIndexedMapperClient()
		const env = "callback-map-test"
		labels := map[string]string{cconfig.EnvironmentLabelKey: env}
		config := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-map-config", Namespace: "default", Labels: labels},
			Spec:       eventv1.EventConfigSpec{Zone: ctypes.ObjectRef{Name: "exposure-zone", Namespace: "default"}},
		}
		exposure := &eventv1.EventExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-map-exposure", Namespace: "default", Labels: map[string]string{
				cconfig.EnvironmentLabelKey: env,
			}},
			Spec: eventv1.EventExposureSpec{
				Zone: ctypes.ObjectRef{Name: "exposure-zone", Namespace: "default"}, EventType: "de.telekom.callbackmap.v1",
				Visibility: eventv1.VisibilityEnterprise, Approval: eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
			},
		}
		remote := &eventv1.EventSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-map-remote", Namespace: "default", Labels: labels},
			Spec: eventv1.EventSubscriptionSpec{
				Zone: ctypes.ObjectRef{Name: "subscriber-zone", Namespace: "default"}, EventType: exposure.Spec.EventType,
				Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Payload: eventv1.PayloadTypeData, Callback: "https://subscriber.example.com/events"},
			},
		}
		unrelated := &eventv1.EventSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-map-unrelated", Namespace: "default", Labels: labels},
			Spec: eventv1.EventSubscriptionSpec{
				Zone: ctypes.ObjectRef{Name: "subscriber-zone", Namespace: "default"}, EventType: "de.telekom.unrelated.v1",
				Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Payload: eventv1.PayloadTypeData, Callback: "https://subscriber.example.com/events"},
			},
		}
		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, exposure)).To(Succeed()) })
		Expect(k8sClient.Create(ctx, remote)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, remote)).To(Succeed()) })
		Expect(k8sClient.Create(ctx, unrelated)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, unrelated)).To(Succeed()) })
		otherEnvSubscription := remote.DeepCopy()
		otherEnvSubscription.Name = "callback-map-other-env"
		otherEnvSubscription.ResourceVersion = ""
		otherEnvSubscription.Labels = map[string]string{cconfig.EnvironmentLabelKey: "other-env"}
		Expect(k8sClient.Create(ctx, otherEnvSubscription)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, otherEnvSubscription)).To(Succeed()) })
		otherEnvExposure := exposure.DeepCopy()
		otherEnvExposure.Name = "callback-map-other-env-exposure"
		otherEnvExposure.ResourceVersion = ""
		otherEnvExposure.Labels = map[string]string{cconfig.EnvironmentLabelKey: "other-env"}
		otherEnvExposure.Spec.EventType = unrelated.Spec.EventType
		Expect(k8sClient.Create(ctx, otherEnvExposure)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, otherEnvExposure)).To(Succeed()) })
		otherNamespaceExposure := exposure.DeepCopy()
		otherNamespaceExposure.Name = "callback-map-other-namespace-exposure"
		otherNamespaceExposure.ResourceVersion = ""
		otherNamespaceExposure.Spec.Zone.Namespace = "other-namespace"
		otherNamespaceExposure.Spec.EventType = unrelated.Spec.EventType
		Expect(k8sClient.Create(ctx, otherNamespaceExposure)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, otherNamespaceExposure)).To(Succeed()) })
		proxyConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "callback-map-proxy", Namespace: "default", Labels: labels},
			Spec: eventv1.EventConfigSpec{
				Zone:  ctypes.ObjectRef{Name: "proxy-exposure", Namespace: "default"},
				Proxy: &eventv1.ProxyBackend{TargetZone: config.Spec.Zone},
			},
		}
		proxyExposure := exposure.DeepCopy()
		proxyExposure.Name = "callback-map-proxy-exposure"
		proxyExposure.ResourceVersion = ""
		proxyExposure.Spec.Zone = proxyConfig.Spec.Zone
		proxyExposure.Spec.EventType = "de.telekom.proxycallbackmap.v1"
		proxySubscription := remote.DeepCopy()
		proxySubscription.Name = "callback-map-proxy-subscription"
		proxySubscription.ResourceVersion = ""
		proxySubscription.Spec.EventType = proxyExposure.Spec.EventType
		proxySubscription.Labels = map[string]string{cconfig.EnvironmentLabelKey: env}
		Expect(k8sClient.Create(ctx, proxyConfig)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, proxyConfig)).To(Succeed()) })
		Expect(k8sClient.Create(ctx, proxyExposure)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, proxyExposure)).To(Succeed()) })
		Expect(k8sClient.Create(ctx, proxySubscription)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, proxySubscription)).To(Succeed()) })
		remoteSSE := remote.DeepCopy()
		remoteSSE.Name = "callback-map-remote-sse"
		remoteSSE.ResourceVersion = ""
		remoteSSE.Spec.Delivery.Type = eventv1.DeliveryTypeServerSentEvent
		remoteSSE.Spec.Delivery.Callback = ""
		Expect(k8sClient.Create(ctx, remoteSSE)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, remoteSSE)).To(Succeed()) })
		localSSE := remoteSSE.DeepCopy()
		localSSE.Name = "callback-map-local-sse"
		localSSE.ResourceVersion = ""
		localSSE.Spec.Zone = config.Spec.Zone
		Expect(k8sClient.Create(ctx, localSSE)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, localSSE)).To(Succeed()) })
		localCallback := remote.DeepCopy()
		localCallback.Name = "callback-map-local-callback"
		localCallback.ResourceVersion = ""
		localCallback.Spec.Zone = config.Spec.Zone
		Expect(k8sClient.Create(ctx, localCallback)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, localCallback)).To(Succeed()) })

		r := &EventSubscriptionReconciler{Client: indexedClient}
		Eventually(func() []reconcile.Request { return r.MapEventConfigToEventSubscription(ctx, config) }, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(
			reconcile.Request{NamespacedName: types.NamespacedName{Name: remote.Name, Namespace: remote.Namespace}},
			reconcile.Request{NamespacedName: types.NamespacedName{Name: localSSE.Name, Namespace: localSSE.Namespace}},
			reconcile.Request{NamespacedName: types.NamespacedName{Name: localCallback.Name, Namespace: localCallback.Namespace}},
		))
		Eventually(func() []reconcile.Request { return r.MapEventConfigToEventSubscription(ctx, proxyConfig) }, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(
			reconcile.Request{NamespacedName: types.NamespacedName{Name: proxySubscription.Name, Namespace: proxySubscription.Namespace}},
		))
		// A projected map entry can change without changing the exposure scalar;
		// the proxy config's status event must still reach remote callbacks.
		proxyConfig.Status.ProxyCallbackURLs = map[string]string{"subscriber-zone": "https://backend/old"}
		Expect(k8sClient.Status().Update(ctx, proxyConfig)).To(Succeed())
		proxyConfig.Status.ProxyCallbackURLs["subscriber-zone"] = "https://backend/new"
		Expect(k8sClient.Status().Update(ctx, proxyConfig)).To(Succeed())
		Eventually(func() []reconcile.Request { return r.MapEventConfigToEventSubscription(ctx, proxyConfig) }, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(
			reconcile.Request{NamespacedName: types.NamespacedName{Name: proxySubscription.Name, Namespace: proxySubscription.Namespace}},
		))
	})

	It("retains same-zone subscriptions when exposure discovery fails", func() {
		indexedClient := newIndexedMapperClient()
		const env = "callback-local-list-failure"
		config := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "local-callback-config", Namespace: "default", Labels: map[string]string{cconfig.EnvironmentLabelKey: env}},
			Spec:       eventv1.EventConfigSpec{Zone: ctypes.ObjectRef{Name: "local-zone", Namespace: "default"}},
		}
		local := &eventv1.EventSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "local-callback-subscription", Namespace: "default", Labels: map[string]string{cconfig.EnvironmentLabelKey: env}},
			Spec: eventv1.EventSubscriptionSpec{
				Zone: ctypes.ObjectRef{Name: "local-zone", Namespace: "default"}, EventType: "de.telekom.callbacklocal.v1",
				Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Payload: eventv1.PayloadTypeData, Callback: "https://subscriber.example.com/events"},
			},
		}
		Expect(k8sClient.Create(ctx, local)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, local)).To(Succeed()) })
		remote := local.DeepCopy()
		remote.Name = "remote-callback-subscription"
		remote.ResourceVersion = ""
		remote.Spec.Zone.Name = "remote-zone"
		Expect(k8sClient.Create(ctx, remote)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, remote)).To(Succeed()) })
		r := &EventSubscriptionReconciler{Client: exposureListFailureClient{Client: indexedClient}}
		Eventually(func() []reconcile.Request { return r.MapEventConfigToEventSubscription(ctx, config) }, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(reconcile.Request{
			NamespacedName: types.NamespacedName{Name: local.Name, Namespace: local.Namespace},
		}))
	})

	It("tracks indexed zone, event type and delivery changes without zone labels", func() {
		indexedClient := newIndexedMapperClient()
		const env = "indexed-mapper-updates"
		labels := map[string]string{cconfig.EnvironmentLabelKey: env}
		zone := ctypes.ObjectRef{Name: "shared-zone", Namespace: "default"}
		config := &eventv1.EventConfig{ObjectMeta: metav1.ObjectMeta{Name: "indexed-config", Namespace: "default", Labels: labels}, Spec: eventv1.EventConfigSpec{Zone: zone}}
		request := func(obj client.Object) reconcile.Request {
			return reconcile.Request{NamespacedName: client.ObjectKeyFromObject(obj)}
		}
		create := func(obj client.Object) {
			ExpectWithOffset(1, k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { ExpectWithOffset(1, k8sClient.Delete(ctx, obj)).To(Succeed()) })
		}
		exposure := &eventv1.EventExposure{ObjectMeta: metav1.ObjectMeta{Name: "indexed-exposure", Namespace: "default", Labels: labels}, Spec: eventv1.EventExposureSpec{Zone: zone, EventType: "de.telekom.indexed.one.v1", Visibility: eventv1.VisibilityEnterprise, Approval: eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto}}}
		create(exposure)
		duplicate := exposure.DeepCopy()
		duplicate.Name = "indexed-exposure-duplicate"
		duplicate.ResourceVersion = ""
		create(duplicate)
		otherZone := exposure.DeepCopy()
		otherZone.Name = "indexed-other-namespace-exposure"
		otherZone.ResourceVersion = ""
		otherZone.Spec.Zone.Namespace = "elsewhere"
		otherZone.Spec.EventType = "de.telekom.indexed.other.v1"
		create(otherZone)
		local := &eventv1.EventSubscription{ObjectMeta: metav1.ObjectMeta{Name: "indexed-local", Namespace: "default", Labels: labels}, Spec: eventv1.EventSubscriptionSpec{Zone: zone, EventType: exposure.Spec.EventType, Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Payload: eventv1.PayloadTypeData, Callback: "https://example.com/events"}}}
		create(local)
		localSSE := local.DeepCopy()
		localSSE.Name = "indexed-local-sse"
		localSSE.ResourceVersion = ""
		localSSE.Spec.Delivery = eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent, Payload: eventv1.PayloadTypeData}
		create(localSSE)
		remote := local.DeepCopy()
		remote.Name = "indexed-remote"
		remote.ResourceVersion = ""
		remote.Spec.Zone.Name = "remote-zone"
		create(remote)
		remoteSSE := remote.DeepCopy()
		remoteSSE.Name = "indexed-remote-sse"
		remoteSSE.ResourceVersion = ""
		remoteSSE.Spec.Delivery = eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent, Payload: eventv1.PayloadTypeData}
		create(remoteSSE)
		wrongNamespace := remote.DeepCopy()
		wrongNamespace.Name = "indexed-wrong-namespace"
		wrongNamespace.ResourceVersion = ""
		wrongNamespace.Spec.Zone = otherZone.Spec.Zone
		wrongNamespace.Spec.EventType = otherZone.Spec.EventType
		create(wrongNamespace)
		wrongNamespaceSSE := localSSE.DeepCopy()
		wrongNamespaceSSE.Name = "indexed-wrong-namespace-sse"
		wrongNamespaceSSE.ResourceVersion = ""
		wrongNamespaceSSE.Spec.Zone.Namespace = "elsewhere"
		create(wrongNamespaceSSE)
		wrongEnv := remote.DeepCopy()
		wrongEnv.Name = "indexed-wrong-environment"
		wrongEnv.ResourceVersion = ""
		wrongEnv.Labels = map[string]string{cconfig.EnvironmentLabelKey: "other-environment"}
		create(wrongEnv)
		wrongEnvExposure := otherZone.DeepCopy()
		wrongEnvExposure.Name = "indexed-wrong-environment-exposure"
		wrongEnvExposure.ResourceVersion = ""
		wrongEnvExposure.Spec.Zone = zone
		wrongEnvExposure.Labels = wrongEnv.Labels
		create(wrongEnvExposure)

		r := &EventSubscriptionReconciler{Client: indexedClient}
		mapped := func() []reconcile.Request { return r.MapEventConfigToEventSubscription(ctx, config) }
		for _, obj := range []client.Object{exposure, duplicate, otherZone, local, localSSE, remote, remoteSSE, wrongNamespace, wrongNamespaceSSE, wrongEnv, wrongEnvExposure} {
			Eventually(func() error {
				return indexedClient.reader.Get(ctx, client.ObjectKeyFromObject(obj), obj.DeepCopyObject().(client.Object))
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		}
		Eventually(mapped, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(request(local), request(localSSE), request(remote)))

		remote.Spec.Delivery.Type = eventv1.DeliveryTypeServerSentEvent
		remote.Spec.Delivery.Callback = ""
		Expect(k8sClient.Update(ctx, remote)).To(Succeed())
		Eventually(mapped, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(request(local), request(localSSE)))
		remote.Spec.Delivery.Type = eventv1.DeliveryTypeCallback
		remote.Spec.Delivery.Callback = "https://example.com/events"
		remote.Spec.EventType = "de.telekom.indexed.two.v1"
		Expect(k8sClient.Update(ctx, remote)).To(Succeed())
		Eventually(func(g Gomega) {
			cached := &eventv1.EventSubscription{}
			g.Expect(indexedClient.reader.Get(ctx, client.ObjectKeyFromObject(remote), cached)).To(Succeed())
			g.Expect(cached.Spec.EventType).To(Equal(remote.Spec.EventType))
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		Expect(mapped()).To(ConsistOf(request(local), request(localSSE)))
		exposure.Spec.EventType = remote.Spec.EventType
		Expect(k8sClient.Update(ctx, exposure)).To(Succeed())
		Eventually(mapped, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(request(local), request(localSSE), request(remote)))
		exposure.Spec.Zone.Name = "another-zone"
		Expect(k8sClient.Update(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) {
			cached := &eventv1.EventExposure{}
			g.Expect(indexedClient.reader.Get(ctx, client.ObjectKeyFromObject(exposure), cached)).To(Succeed())
			g.Expect(cached.Spec.Zone).To(Equal(exposure.Spec.Zone))
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		Eventually(mapped, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(request(local), request(localSSE)))
		remote.Spec.Zone = zone
		Expect(k8sClient.Update(ctx, remote)).To(Succeed())
		Eventually(mapped, 10*time.Second, 100*time.Millisecond).Should(ConsistOf(request(local), request(localSSE), request(remote)))
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		subscriptionCtx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		eventsubscriptionObj := &eventv1.EventSubscription{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind EventSubscription")
			err := k8sClient.Get(subscriptionCtx, typeNamespacedName, eventsubscriptionObj)
			if err != nil && errors.IsNotFound(err) {
				resource := &eventv1.EventSubscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: eventv1.EventSubscriptionSpec{
						EventType: "de.telekom.test.v1",
						Zone:      ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
						Requestor: ctypes.TypedObjectRef{
							TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
							ObjectRef: ctypes.ObjectRef{Name: "test-app", Namespace: "default"},
						},
						Delivery: eventv1.Delivery{
							Type:     eventv1.DeliveryTypeCallback,
							Payload:  eventv1.PayloadTypeData,
							Callback: "https://callback.example.com",
						},
					},
				}
				Expect(k8sClient.Create(subscriptionCtx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &eventv1.EventSubscription{}
			err := k8sClient.Get(subscriptionCtx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance EventSubscription")
			Expect(k8sClient.Delete(subscriptionCtx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &EventSubscriptionReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			controllerReconciler.Controller = cc.NewController(&eventsubscription.EventSubscriptionHandler{}, k8sClient, recorder)

			_, err := controllerReconciler.Reconcile(subscriptionCtx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
