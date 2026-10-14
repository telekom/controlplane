// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"path/filepath"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Remote subscription cleanup", func() {
	It("preserves a shared proxy route and another subscription's resources", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		testEnv := &envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join("..", "..", "..", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "..", "gateway", "config", "crd", "bases"),
			},
			ErrorIfCRDPathMissing: true,
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
		scheme := runtime.NewScheme()
		Expect(apiapi.AddToScheme(scheme)).To(Succeed())
		Expect(gatewayapi.AddToScheme(scheme)).To(Succeed())
		apiClient, err := client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		objectCache, err := cache.New(cfg, cache.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(index.SetOwnerIndex(ctx, objectCache, &apiapi.RemoteApiSubscription{})).To(Succeed())
		Expect(index.SetOwnerIndex(ctx, objectCache, &gatewayapi.ConsumeRoute{})).To(Succeed())
		cacheDone := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(cacheDone)
			Expect(objectCache.Start(ctx)).To(Succeed())
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(cacheDone, 5*time.Second, 100*time.Millisecond).Should(BeClosed())
		})
		Expect(objectCache.WaitForCacheSync(ctx)).To(BeTrue())
		cachedClient, err := client.New(cfg, client.Options{
			Scheme: scheme, Cache: &client.CacheOptions{Reader: objectCache},
		})
		Expect(err).NotTo(HaveOccurred())

		subscriptions := make([]*apiapi.ApiSubscription, 0, 2)
		requests := make([]*apiapi.RemoteApiSubscription, 0, 2)
		consumeRoutes := make([]*gatewayapi.ConsumeRoute, 0, 2)
		routeRef := types.ObjectRef{Name: "shared-proxy", Namespace: "default"}
		for _, name := range []string{"invalid", "valid"} {
			sub := &apiapi.ApiSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec: apiapi.ApiSubscriptionSpec{
					ApiBasePath: "/shared/v1", Organization: "remote",
					Zone:      types.ObjectRef{Name: "downstream", Namespace: "default"},
					Requestor: apiapi.Requestor{Application: types.ObjectRef{Name: name, Namespace: "default"}},
					Security: &apiapi.SubscriberSecurity{M2M: &apiapi.SubscriberMachine2MachineAuthentication{
						Scopes: []string{"read"},
					}},
				},
			}
			Expect(apiClient.Create(ctx, sub)).To(Succeed())
			request := &apiapi.RemoteApiSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: "default",
					Labels: map[string]string{config.EnvironmentLabelKey: "test"},
				},
				Spec: apiapi.RemoteApiSubscriptionSpec{
					ApiBasePath: "/shared/v1", TargetOrganization: "remote",
					Requester: apiapi.RemoteRequester{Application: name},
				},
			}
			Expect(controllerutil.SetControllerReference(sub, request, scheme)).To(Succeed())
			Expect(apiClient.Create(ctx, request)).To(Succeed())
			consumeRoute := &gatewayapi.ConsumeRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: "default",
					Labels: map[string]string{config.EnvironmentLabelKey: "test"},
				},
				Spec: gatewayapi.ConsumeRouteSpec{Route: routeRef, ConsumerName: name},
			}
			Expect(controllerutil.SetControllerReference(sub, consumeRoute, scheme)).To(Succeed())
			Expect(apiClient.Create(ctx, consumeRoute)).To(Succeed())
			sub.Status.Route = &routeRef
			sub.Status.RemoteApiSubscription = types.ObjectRefFromObject(request)
			sub.Status.ConsumeRoute = types.ObjectRefFromObject(consumeRoute)
			Expect(apiClient.Status().Update(ctx, sub)).To(Succeed())
			subscriptions = append(subscriptions, sub)
			requests = append(requests, request)
			consumeRoutes = append(consumeRoutes, consumeRoute)
		}
		proxyRoute := &gatewayapi.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: routeRef.Name, Namespace: routeRef.Namespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: "test",
					config.OwnerUidLabelKey:    string(subscriptions[0].UID),
				},
			},
			Spec: gatewayapi.RouteSpec{
				Type: gatewayapi.RouteTypeProxy, GatewayRef: types.ObjectRef{Name: "gateway", Namespace: "default"},
				Backend: gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{{
					Scheme: "https", Hostname: "upstream.example.com", Port: 443, Path: "/shared/v1",
				}}},
			},
		}
		Expect(apiClient.Create(ctx, proxyRoute)).To(Succeed())
		Eventually(func(g Gomega) {
			for i := range requests {
				g.Expect(cachedClient.Get(ctx, client.ObjectKeyFromObject(requests[i]), &apiapi.RemoteApiSubscription{})).To(Succeed())
				g.Expect(cachedClient.Get(ctx, client.ObjectKeyFromObject(consumeRoutes[i]), &gatewayapi.ConsumeRoute{})).To(Succeed())
			}
			g.Expect(cachedClient.Get(ctx, routeRef.K8s(), &gatewayapi.Route{})).To(Succeed())
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		subscriptions[0].Spec.Security.M2M.Client = &apiapi.OAuth2ClientCredentials{ClientId: "client", ClientSecret: "secret"}
		Expect(apiClient.Update(ctx, subscriptions[0])).To(Succeed())
		handlerCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(cachedClient, "test")))
		Expect(handleRemoteApiSubscription(handlerCtx, subscriptions[0])).To(Succeed())
		Expect(subscriptions[0].Status.Route).To(BeNil())
		Expect(subscriptions[0].Status.RemoteApiSubscription).To(BeNil())
		Expect(subscriptions[0].Status.ConsumeRoute).To(BeNil())
		Expect(apiClient.Get(ctx, client.ObjectKeyFromObject(requests[0]), &apiapi.RemoteApiSubscription{})).To(
			WithTransform(apierrors.IsNotFound, BeTrue()),
		)
		Expect(apiClient.Get(ctx, client.ObjectKeyFromObject(consumeRoutes[0]), &gatewayapi.ConsumeRoute{})).To(
			WithTransform(apierrors.IsNotFound, BeTrue()),
		)
		remainingRoute := &gatewayapi.Route{}
		Expect(apiClient.Get(ctx, routeRef.K8s(), remainingRoute)).To(Succeed())
		Expect(remainingRoute.UID).To(Equal(proxyRoute.UID))
		remainingConsumeRoute := &gatewayapi.ConsumeRoute{}
		Expect(apiClient.Get(ctx, client.ObjectKeyFromObject(consumeRoutes[1]), remainingConsumeRoute)).To(Succeed())
		Expect(remainingConsumeRoute.Spec.Route).To(Equal(routeRef))
		Expect(apiClient.Get(ctx, client.ObjectKeyFromObject(requests[1]), &apiapi.RemoteApiSubscription{})).To(Succeed())
		remainingSubscription := &apiapi.ApiSubscription{}
		Expect(apiClient.Get(ctx, client.ObjectKeyFromObject(subscriptions[1]), remainingSubscription)).To(Succeed())
		Expect(remainingSubscription.Status).To(Equal(subscriptions[1].Status))
	})
})
