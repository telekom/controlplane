// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
// Copyright 2026.
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Listener Mapper Tests", Ordered, func() {
	const (
		mapEnv   = "mapper-env"
		mapNs    = "mapper-ns"
		zoneNs   = "mapper-env--aws"
		appName  = "mapper-consumer"
		provName = "mapper-provider"
		saName   = "mapper-sa"
		basePath = "/api/v1/test"
	)

	var (
		ctx        context.Context
		reconciler *ListenerReconciler
	)

	BeforeAll(func() {
		ctx = context.Background()

		recorder := record.NewFakeRecorder(10)
		reconciler = &ListenerReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: recorder,
		}
		reconciler.Controller = cc.NewController(&handler.ListenerHandler{}, k8sClient, recorder)

		// Create necessary namespaces for mapper tests.
		for _, ns := range []string{mapNs, zoneNs} {
			nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, nsObj))).To(Succeed())
		}

		// Create a Listener for mapper tests.
		listener := &spectrev1.Listener{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mapper-listener",
				Namespace: mapNs,
				Labels:    map[string]string{envLabelKey: mapEnv},
			},
			Spec: spectrev1.ListenerSpec{
				Consumer: ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
					ObjectRef: ctypes.ObjectRef{Name: appName, Namespace: mapNs},
				},
				Provider: ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
					ObjectRef: ctypes.ObjectRef{Name: provName, Namespace: mapNs},
				},
				Application: ctypes.ObjectRef{Name: saName, Namespace: mapNs},
				ApiListener: &spectrev1.ApiListener{ApiBasePath: basePath},
			},
		}
		Expect(k8sClient.Create(ctx, listener)).To(Succeed())

		// Wait for Listener to be visible in cache.
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(listener), &spectrev1.Listener{})
		}, testTimeout, testInterval).Should(Succeed())
	})

	Describe("mapSpectreApplicationToListeners", func() {
		It("should match when SpectreApplication ref equals spec.application", func() {
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      saName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapSpectreApplicationToListeners(ctx, sa)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match when SpectreApplication name differs", func() {
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-sa",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapSpectreApplicationToListeners(ctx, sa)
			Expect(reqs).To(BeEmpty())
		})

		It("should not match a different environment", func() {
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      saName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: "other-env"},
				},
			}
			reqs := reconciler.mapSpectreApplicationToListeners(ctx, sa)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapOwnedChildToListener", func() {
		It("should match when child has OwnerUidLabelKey matching Listener UID", func() {
			// Get the actual Listener UID.
			listener := &spectrev1.Listener{}
			Expect(directClient.Get(ctx, types.NamespacedName{Name: "mapper-listener", Namespace: mapNs}, listener)).To(Succeed())

			child := &gatewayv1.RouteListener{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owned-rl",
					Namespace: zoneNs,
					Labels: map[string]string{
						cconfig.OwnerUidLabelKey: string(listener.UID),
					},
				},
			}
			reqs := reconciler.mapOwnedChildToListener(ctx, child)
			Expect(reqs).To(HaveLen(1))
			Expect(reqs[0].NamespacedName.Name).To(Equal("mapper-listener"))
		})

		It("should not match when OwnerUidLabelKey is absent", func() {
			child := &pubsubv1.Subscriber{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-label-sub",
					Namespace: zoneNs,
				},
			}
			reqs := reconciler.mapOwnedChildToListener(ctx, child)
			Expect(reqs).To(BeEmpty())
		})

		It("should not match when OwnerUidLabelKey has a different UID", func() {
			child := &pubsubv1.Subscriber{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-uid-sub",
					Namespace: zoneNs,
					Labels: map[string]string{
						cconfig.OwnerUidLabelKey: "00000000-0000-0000-0000-000000000000",
					},
				},
			}
			reqs := reconciler.mapOwnedChildToListener(ctx, child)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapRouteToListeners", func() {
		It("should match when Route name equals normalized apiBasePath", func() {
			route := &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      labelutil.NormalizeValue(basePath),
					Namespace: zoneNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapRouteToListeners(ctx, route)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match when Route name differs", func() {
			route := &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-route",
					Namespace: zoneNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapRouteToListeners(ctx, route)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapApplicationToListeners", func() {
		It("should match when Application is the consumer", func() {
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapApplicationToListeners(ctx, app)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should match when Application is the provider", func() {
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      provName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapApplicationToListeners(ctx, app)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match an unrelated Application", func() {
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated-app",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapApplicationToListeners(ctx, app)
			Expect(reqs).To(BeEmpty())
		})

		It("should match when Application is the observer via SpectreApplication", func() {
			// The Listener has spec.application = {name: mapper-sa, namespace: mapper-ns}.
			// Create a SpectreApplication with that name whose spec.application
			// references a third Application (the observer's real Application).
			observerAppName := "mapper-observer-app"
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      saName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: spectrev1.SpectreApplicationSpec{
					Application: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: observerAppName, Namespace: mapNs},
					},
					DeliveryType: "server_sent_event",
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, sa))).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(sa), &spectrev1.SpectreApplication{})
			}, testTimeout, testInterval).Should(Succeed())

			// A change to the observer Application should wake the Listener.
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      observerAppName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapApplicationToListeners(ctx, app)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not duplicate when Application is both consumer and observer", func() {
			// The consumer Application is already appName. If a SpectreApplication
			// also references the same Application, the mapper should not return
			// duplicates.
			sa := &spectrev1.SpectreApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sa-dup-test",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: spectrev1.SpectreApplicationSpec{
					Application: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: appName, Namespace: mapNs},
					},
					DeliveryType: "server_sent_event",
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, sa))).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(sa), &spectrev1.SpectreApplication{})
			}, testTimeout, testInterval).Should(Succeed())

			// Create a second listener that references sa-dup-test as its application
			// AND uses appName as consumer.
			dupListener := &spectrev1.Listener{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mapper-listener-dup",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: spectrev1.ListenerSpec{
					Consumer: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: appName, Namespace: mapNs},
					},
					Provider: ctypes.TypedObjectRef{
						TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
						ObjectRef: ctypes.ObjectRef{Name: provName, Namespace: mapNs},
					},
					Application: ctypes.ObjectRef{Name: "sa-dup-test", Namespace: mapNs},
					ApiListener: &spectrev1.ApiListener{ApiBasePath: "/api/v1/dup"},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, dupListener))).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(dupListener), &spectrev1.Listener{})
			}, testTimeout, testInterval).Should(Succeed())

			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapApplicationToListeners(ctx, app)
			// Count how many times mapper-listener-dup appears — should be exactly 1.
			dupCount := 0
			for _, r := range reqs {
				if r.NamespacedName.Name == "mapper-listener-dup" {
					dupCount++
				}
			}
			Expect(dupCount).To(Equal(1), "Listener should appear only once even when matched via both consumer and observer path")
		})
	})

	Describe("mapApiExposureToListeners", func() {
		It("should match when ApiExposure status.route corresponds to the Listener's apiBasePath", func() {
			routeName := labelutil.NormalizeValue(basePath)
			exposure := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exposure-for-mapper",
					Namespace: "zone-ns",
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Status: apiv1.ApiExposureStatus{
					Active: true,
					Route:  &ctypes.ObjectRef{Name: routeName, Namespace: "zone-ns"},
				},
			}
			reqs := reconciler.mapApiExposureToListeners(ctx, exposure)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should match via proxyRoutes as well", func() {
			routeName := labelutil.NormalizeValue(basePath)
			exposure := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exposure-proxy",
					Namespace: "zone-ns",
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Status: apiv1.ApiExposureStatus{
					Active:      true,
					ProxyRoutes: []ctypes.ObjectRef{{Name: routeName, Namespace: "zone-ns"}},
				},
			}
			reqs := reconciler.mapApiExposureToListeners(ctx, exposure)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match when ApiExposure has no route refs", func() {
			exposure := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exposure-no-route",
					Namespace: "zone-ns",
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Status: apiv1.ApiExposureStatus{Active: false},
			}
			reqs := reconciler.mapApiExposureToListeners(ctx, exposure)
			Expect(reqs).To(BeEmpty())
		})

		It("should not match when route name does not correspond to any Listener's apiBasePath", func() {
			exposure := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exposure-other-route",
					Namespace: "zone-ns",
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Status: apiv1.ApiExposureStatus{
					Active: true,
					Route:  &ctypes.ObjectRef{Name: "unrelated-route", Namespace: "zone-ns"},
				},
			}
			reqs := reconciler.mapApiExposureToListeners(ctx, exposure)
			Expect(reqs).To(BeEmpty())
		})

		It("should not match when environment differs", func() {
			routeName := labelutil.NormalizeValue(basePath)
			exposure := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exposure-other-env",
					Namespace: "zone-ns",
					Labels:    map[string]string{envLabelKey: "other-env"},
				},
				Status: apiv1.ApiExposureStatus{
					Active: true,
					Route:  &ctypes.ObjectRef{Name: routeName, Namespace: "zone-ns"},
				},
			}
			reqs := reconciler.mapApiExposureToListeners(ctx, exposure)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapZoneToListeners", func() {
		It("should match when Zone is referenced by the consumer Application", func() {
			// Create the consumer Application with a Zone reference.
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: applicationv1.ApplicationSpec{
					Team:      "team-a",
					TeamEmail: "a@test.com",
					Secret:    "s",
					Zone:      ctypes.ObjectRef{Name: "zone-a", Namespace: mapNs},
					Failover:  applicationv1.Failover{Enabled: false},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, app))).To(Succeed())

			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &applicationv1.Application{})
			}, testTimeout, testInterval).Should(Succeed())

			zone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zone-a",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapZoneToListeners(ctx, zone)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match an unrelated Zone", func() {
			zone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zone-unrelated",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapZoneToListeners(ctx, zone)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapEventConfigToListeners", func() {
		It("should match when Listener's Application is in the EventConfig's zone", func() {
			// The consumer Application mapper-consumer was created in mapZoneToListeners
			// with zone ref zone-a/mapper-ns.
			ec := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ec-zone-a",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "zone-a", Namespace: mapNs},
					Local: &eventv1.LocalBackend{
						Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
						ServerSendEventUrl: "https://sse.local",
						PublishEventUrl:    "http://publish.local",
					},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ec))).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(ec), &eventv1.EventConfig{})
			}, testTimeout, testInterval).Should(Succeed())

			reqs := reconciler.mapEventConfigToListeners(ctx, ec)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match when no Applications are in the EventConfig's zone", func() {
			ec := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ec-missing",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "nonexistent-zone", Namespace: mapNs},
				},
			}
			reqs := reconciler.mapEventConfigToListeners(ctx, ec)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapEventStoreToListeners", func() {
		It("should match when EventConfig references the EventStore and Listener is in that zone", func() {
			// Create an EventConfig referencing an EventStore and zone-a.
			ec := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ec-with-listener-es",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "zone-a", Namespace: mapNs},
					Local: &eventv1.LocalBackend{
						Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
						ServerSendEventUrl: "https://sse.local",
						PublishEventUrl:    "http://publish.local",
					},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ec))).To(Succeed())

			// Set EventStore ref in status.
			Eventually(func(g Gomega) {
				fetched := &eventv1.EventConfig{}
				g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(ec), fetched)).To(Succeed())
				fetched.Status.EventStore = &ctypes.ObjectRef{Name: "listener-es", Namespace: mapNs}
				g.Expect(directClient.Status().Update(ctx, fetched)).To(Succeed())
			}, testTimeout, testInterval).Should(Succeed())

			Eventually(func(g Gomega) {
				cached := &eventv1.EventConfig{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ec), cached)).To(Succeed())
				g.Expect(cached.Status.EventStore).NotTo(BeNil())
			}, testTimeout, testInterval).Should(Succeed())

			es := &pubsubv1.EventStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "listener-es",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapEventStoreToListeners(ctx, es)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match when no EventConfig references the EventStore", func() {
			es := &pubsubv1.EventStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-listener-es",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapEventStoreToListeners(ctx, es)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapGenericPublisherToListeners", func() {
		It("should match all Listeners in the same environment for the generic Publisher", func() {
			pub := &pubsubv1.Publisher{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.MakePublisherName(util.GenericEventType),
					Namespace: zoneNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapGenericPublisherToListeners(ctx, pub)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match for a non-generic Publisher", func() {
			pub := &pubsubv1.Publisher{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-other-publisher",
					Namespace: zoneNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapGenericPublisherToListeners(ctx, pub)
			Expect(reqs).To(BeEmpty())
		})

		It("should not match when Publisher has a different environment", func() {
			pub := &pubsubv1.Publisher{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.MakePublisherName(util.GenericEventType),
					Namespace: zoneNs,
					Labels:    map[string]string{envLabelKey: "other-env"},
				},
			}
			reqs := reconciler.mapGenericPublisherToListeners(ctx, pub)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapRealmToListeners", func() {
		It("should match when Realm is referenced by a Zone containing the consumer Application", func() {
			// Create a Zone with status.identityRealm pointing at our realm.
			zone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zone-a",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
				Spec: adminv1.ZoneSpec{
					IdentityProvider: adminv1.IdentityProviderConfig{
						Url:   "http://id.local",
						Admin: adminv1.IdentityProviderAdminConfig{ClientId: "a", UserName: "a", Password: "a"},
					},
					Gateway: adminv1.GatewayConfig{
						Admin: adminv1.GatewayAdminConfig{Url: "http://gw.local"},
						Presets: []adminv1.GatewayConfigPreset{{
							Name: "default", Default: true,
							Urls: []adminv1.UrlConfig{{Hostname: "gw.test.local", BasePath: "/"}},
						}},
					},
					Visibility: adminv1.ZoneVisibilityWorld,
				},
			}
			Expect(client.IgnoreAlreadyExists(directClient.Create(ctx, zone))).To(Succeed())

			// Set status.identityRealm with valid Links URIs.
			Eventually(func(g Gomega) {
				fetched := &adminv1.Zone{}
				g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(zone), fetched)).To(Succeed())
				fetched.Status.IdentityRealm = &ctypes.ObjectRef{Name: "mapper-realm", Namespace: mapNs}
				fetched.Status.Namespace = zoneNs
				fetched.Status.Links = adminv1.Links{
					Url:       "http://gw.test.local",
					Issuer:    "http://id.local/realms/mapper-env",
					LmsIssuer: "http://id.local/realms/mapper-env-lms",
				}
				g.Expect(directClient.Status().Update(ctx, fetched)).To(Succeed())
			}, testTimeout, testInterval).Should(Succeed())

			Eventually(func(g Gomega) {
				cached := &adminv1.Zone{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), cached)).To(Succeed())
				g.Expect(cached.Status.IdentityRealm).NotTo(BeNil())
			}, testTimeout, testInterval).Should(Succeed())

			realm := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mapper-realm",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapRealmToListeners(ctx, realm)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "mapper-listener", Namespace: mapNs},
			}))
		})

		It("should not match when Realm is not referenced by any Zone", func() {
			realm := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unreferenced-realm",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: mapEnv},
				},
			}
			reqs := reconciler.mapRealmToListeners(ctx, realm)
			Expect(reqs).To(BeEmpty())
		})

		It("should not match when Realm is in a different environment", func() {
			realm := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mapper-realm",
					Namespace: mapNs,
					Labels:    map[string]string{envLabelKey: "other-env"},
				},
			}
			reqs := reconciler.mapRealmToListeners(ctx, realm)
			Expect(reqs).To(BeEmpty())
		})
	})
})

var _ = Describe("SpectreApplication Mapper Tests", Ordered, func() {
	const (
		saMapEnv = "sa-mapper-env"
		saMapNs  = "sa-mapper-ns"
		saAppRef = "sa-app-ref"
	)

	var (
		ctx        context.Context
		reconciler *SpectreApplicationReconciler
	)

	BeforeAll(func() {
		ctx = context.Background()

		recorder := record.NewFakeRecorder(10)
		reconciler = &SpectreApplicationReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: recorder,
		}
		reconciler.Controller = cc.NewController(&handler.SpectreApplicationHandler{}, k8sClient, recorder)

		// Create necessary namespaces.
		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: saMapNs}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, nsObj))).To(Succeed())

		// Create a SpectreApplication for mapper tests.
		sa := &spectrev1.SpectreApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sa-mapper-target",
				Namespace: saMapNs,
				Labels:    map[string]string{envLabelKey: saMapEnv},
			},
			Spec: spectrev1.SpectreApplicationSpec{
				Application: ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
					ObjectRef: ctypes.ObjectRef{Name: saAppRef, Namespace: saMapNs},
				},
				DeliveryType: "server_sent_event",
			},
		}
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())

		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(sa), &spectrev1.SpectreApplication{})
		}, testTimeout, testInterval).Should(Succeed())
	})

	Describe("mapOwnedChildToSpectreApplication", func() {
		It("should match when child has OwnerUidLabelKey matching SpectreApplication UID", func() {
			sa := &spectrev1.SpectreApplication{}
			Expect(directClient.Get(ctx, types.NamespacedName{Name: "sa-mapper-target", Namespace: saMapNs}, sa)).To(Succeed())

			child := &pubsubv1.Publisher{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owned-pub",
					Namespace: "zone-ns",
					Labels: map[string]string{
						cconfig.OwnerUidLabelKey: string(sa.UID),
					},
				},
			}
			reqs := reconciler.mapOwnedChildToSpectreApplication(ctx, child)
			Expect(reqs).To(HaveLen(1))
			Expect(reqs[0].NamespacedName.Name).To(Equal("sa-mapper-target"))
		})

		It("should not match when OwnerUidLabelKey is absent", func() {
			child := &pubsubv1.Subscriber{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-label",
					Namespace: "zone-ns",
				},
			}
			reqs := reconciler.mapOwnedChildToSpectreApplication(ctx, child)
			Expect(reqs).To(BeEmpty())
		})

		It("should not match a non-existent UID", func() {
			child := &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-uid",
					Namespace: "zone-ns",
					Labels: map[string]string{
						cconfig.OwnerUidLabelKey: "nonexistent-uid",
					},
				},
			}
			reqs := reconciler.mapOwnedChildToSpectreApplication(ctx, child)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapApplicationToSpectreApplications", func() {
		It("should match when Application matches spec.application", func() {
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      saAppRef,
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
			}
			reqs := reconciler.mapApplicationToSpectreApplications(ctx, app)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "sa-mapper-target", Namespace: saMapNs},
			}))
		})

		It("should not match an unrelated Application", func() {
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated",
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
			}
			reqs := reconciler.mapApplicationToSpectreApplications(ctx, app)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapZoneToSpectreApplications", func() {
		It("should match when Zone is referenced by the SA's Application", func() {
			// Create the Application with a zone ref.
			app := &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      saAppRef,
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
				Spec: applicationv1.ApplicationSpec{
					Team:      "t",
					TeamEmail: "e@e.com",
					Secret:    "s",
					Zone:      ctypes.ObjectRef{Name: "sa-zone", Namespace: saMapNs},
					Failover:  applicationv1.Failover{Enabled: false},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, app))).To(Succeed())

			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &applicationv1.Application{})
			}, testTimeout, testInterval).Should(Succeed())

			zone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sa-zone",
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
			}
			reqs := reconciler.mapZoneToSpectreApplications(ctx, zone)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "sa-mapper-target", Namespace: saMapNs},
			}))
		})

		It("should not match an unrelated Zone", func() {
			zone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated-zone",
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
			}
			reqs := reconciler.mapZoneToSpectreApplications(ctx, zone)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapEventConfigToSpectreApplications", func() {
		It("should match when SA's Application is in the EventConfig's zone", func() {
			// The Application sa-app-ref was created in mapZoneToSpectreApplications
			// with zone ref sa-zone/sa-mapper-ns. Create an EventConfig for that zone.
			ec := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ec-sa-zone",
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "sa-zone", Namespace: saMapNs},
					Local: &eventv1.LocalBackend{
						Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
						ServerSendEventUrl: "https://sse.local",
						PublishEventUrl:    "http://publish.local",
					},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ec))).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(ec), &eventv1.EventConfig{})
			}, testTimeout, testInterval).Should(Succeed())

			reqs := reconciler.mapEventConfigToSpectreApplications(ctx, ec)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "sa-mapper-target", Namespace: saMapNs},
			}))
		})

		It("should not match when no Applications are in the EventConfig's zone", func() {
			ec := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ec-no-zone",
					Namespace: "other-ns",
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "missing-zone", Namespace: saMapNs},
				},
			}
			reqs := reconciler.mapEventConfigToSpectreApplications(ctx, ec)
			Expect(reqs).To(BeEmpty())
		})
	})

	Describe("mapEventStoreToSpectreApplications", func() {
		It("should match when EventConfig references the EventStore and SA is in that zone", func() {
			// Create an EventConfig that references an EventStore and is in sa-zone.
			ec := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ec-with-es",
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "sa-zone", Namespace: saMapNs},
					Local: &eventv1.LocalBackend{
						Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
						ServerSendEventUrl: "https://sse.local",
						PublishEventUrl:    "http://publish.local",
					},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ec))).To(Succeed())

			// Set EventStore ref in status.
			Eventually(func(g Gomega) {
				fetched := &eventv1.EventConfig{}
				g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(ec), fetched)).To(Succeed())
				fetched.Status.EventStore = &ctypes.ObjectRef{Name: "sa-es", Namespace: saMapNs}
				g.Expect(directClient.Status().Update(ctx, fetched)).To(Succeed())
			}, testTimeout, testInterval).Should(Succeed())

			// Wait for cache to reflect status update.
			Eventually(func(g Gomega) {
				cached := &eventv1.EventConfig{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ec), cached)).To(Succeed())
				g.Expect(cached.Status.EventStore).NotTo(BeNil())
			}, testTimeout, testInterval).Should(Succeed())

			es := &pubsubv1.EventStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sa-es",
					Namespace: saMapNs,
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
			}
			reqs := reconciler.mapEventStoreToSpectreApplications(ctx, es)
			Expect(reqs).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "sa-mapper-target", Namespace: saMapNs},
			}))
		})

		It("should not match when no EventConfig references the EventStore", func() {
			es := &pubsubv1.EventStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-es",
					Namespace: "other-ns",
					Labels:    map[string]string{envLabelKey: saMapEnv},
				},
			}
			reqs := reconciler.mapEventStoreToSpectreApplications(ctx, es)
			Expect(reqs).To(BeEmpty())
		})
	})
})

var _ = Describe("Listener dependency mapping (observer, applied, draining)", Ordered, func() {
	const (
		topoEnv   = "topo-env"
		topoOther = "topo-other-env"
		topoNs    = "topo-ns"
	)

	var (
		ctx        context.Context
		reconciler *ListenerReconciler
	)

	key := func(name string) types.NamespacedName { return types.NamespacedName{Name: name, Namespace: topoNs} }
	ref := func(name string) *ctypes.ObjectRef { return &ctypes.ObjectRef{Name: name, Namespace: topoNs} }
	req := func(name string) reconcile.Request { return reconcile.Request{NamespacedName: key(name)} }
	meta := func(name, env string) metav1.ObjectMeta {
		m := metav1.ObjectMeta{Name: name, Namespace: topoNs}
		if env != "" {
			m.Labels = map[string]string{envLabelKey: env}
		}
		return m
	}
	zoneObj := func(name, env string) *adminv1.Zone { return &adminv1.Zone{ObjectMeta: meta(name, env)} }
	ecObj := func(zone, env string) *eventv1.EventConfig {
		return &eventv1.EventConfig{ObjectMeta: meta("ec-"+zone, env), Spec: eventv1.EventConfigSpec{Zone: *ref(zone)}}
	}
	esObj := func(name, env string) *pubsubv1.EventStore { return &pubsubv1.EventStore{ObjectMeta: meta(name, env)} }
	routeObj := func(name, env string) *gatewayv1.Route { return &gatewayv1.Route{ObjectMeta: meta(name, env)} }
	appRef := func(name string) ctypes.TypedObjectRef {
		return ctypes.TypedObjectRef{
			TypeMeta:  metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
			ObjectRef: *ref(name),
		}
	}
	countOf := func(reqs []reconcile.Request, name string) int {
		n := 0
		for _, r := range reqs {
			if r.NamespacedName == key(name) {
				n++
			}
		}
		return n
	}

	BeforeAll(func() {
		ctx = context.Background()

		recorder := record.NewFakeRecorder(10)
		reconciler = &ListenerReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: recorder,
		}
		reconciler.Controller = cc.NewController(&handler.ListenerHandler{}, k8sClient, recorder)

		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: topoNs}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, nsObj))).To(Succeed())

		// C and P share zone-cap; A is alone in zone-obs. No status, so the
		// running Listener controller blocks at resolveApplication and never
		// touches the applied placement written below.
		for name, zone := range map[string]string{"topo-c": "zone-cap", "topo-p": "zone-cap", "topo-a": "zone-obs"} {
			app := &applicationv1.Application{
				ObjectMeta: meta(name, topoEnv),
				Spec: applicationv1.ApplicationSpec{
					Team: "team-" + name, TeamEmail: name + "@test.com", Secret: "s",
					Zone:     *ref(zone),
					Failover: applicationv1.Failover{Enabled: false},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, key(name), &applicationv1.Application{})
			}, testTimeout, testInterval).Should(Succeed())
		}

		sa := &spectrev1.SpectreApplication{
			ObjectMeta: meta("topo-sa", topoEnv),
			Spec:       spectrev1.SpectreApplicationSpec{Application: appRef("topo-a"), DeliveryType: "server_sent_event"},
		}
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, key("topo-sa"), &spectrev1.SpectreApplication{})
		}, testTimeout, testInterval).Should(Succeed())

		// A's EventConfig proxies to zone-backend and uses EventStore es-obs.
		ec := &eventv1.EventConfig{
			ObjectMeta: meta("ec-obs", topoEnv),
			Spec: eventv1.EventConfigSpec{
				Zone:  *ref("zone-obs"),
				Proxy: &eventv1.ProxyBackend{TargetZone: *ref("zone-backend")},
			},
		}
		Expect(k8sClient.Create(ctx, ec)).To(Succeed())
		Eventually(func(g Gomega) {
			fetched := &eventv1.EventConfig{}
			g.Expect(directClient.Get(ctx, key("ec-obs"), fetched)).To(Succeed())
			fetched.Status.EventStore = ref("es-obs")
			g.Expect(directClient.Status().Update(ctx, fetched)).To(Succeed())
		}, testTimeout, testInterval).Should(Succeed())
		Eventually(func(g Gomega) {
			cached := &eventv1.EventConfig{}
			g.Expect(k8sClient.Get(ctx, key("ec-obs"), cached)).To(Succeed())
			g.Expect(cached.Status.EventStore).NotTo(BeNil())
		}, testTimeout, testInterval).Should(Succeed())

		createListener := func(name, consumer, provider, spectreApp, path string, ap *spectrev1.AppliedListenerPlacementStatus) {
			l := &spectrev1.Listener{
				ObjectMeta: meta(name, topoEnv),
				Spec: spectrev1.ListenerSpec{
					Consumer:    appRef(consumer),
					Provider:    appRef(provider),
					Application: *ref(spectreApp),
					ApiListener: &spectrev1.ApiListener{ApiBasePath: path},
				},
			}
			Expect(k8sClient.Create(ctx, l)).To(Succeed())
			if ap != nil {
				Eventually(func(g Gomega) {
					fetched := &spectrev1.Listener{}
					g.Expect(directClient.Get(ctx, key(name), fetched)).To(Succeed())
					fetched.Status.AppliedPlacement = ap.DeepCopy()
					g.Expect(directClient.Status().Update(ctx, fetched)).To(Succeed())
				}, testTimeout, testInterval).Should(Succeed())
			}
			Eventually(func(g Gomega) {
				cached := &spectrev1.Listener{}
				g.Expect(k8sClient.Get(ctx, key(name), cached)).To(Succeed())
				if ap != nil {
					g.Expect(cached.Status.AppliedPlacement).NotTo(BeNil())
				}
			}, testTimeout, testInterval).Should(Succeed())
		}
		createListener("topo-listener", "topo-c", "topo-p", "topo-sa", "/api/v1/topo", nil)
		// Spec refs name objects that do not exist: only the status can match.
		createListener("topo-applied", "topo-gone-c", "topo-gone-p", "topo-gone-sa", "/api/v1/topo-applied",
			&spectrev1.AppliedListenerPlacementStatus{
				Fingerprint:        "fp-old",
				CaptureZone:        ref("zone-old-cap"),
				DeliveryZone:       ref("zone-old-del"),
				CallbackOriginZone: ref("zone-old-origin"),
				CaptureEventStore:  ref("es-old-cap"),
				DeliveryEventStore: ref("es-old-del"),
				CaptureRoute:       ref("route-old"),
				Publisher:          &ctypes.ObjectRef{Name: util.MakePublisherName(util.GenericEventType), Namespace: "old-zone-ns"},
			})
		// Consumer, observer and applied delivery zone all hit zone-obs.
		createListener("topo-dedupe", "topo-a", "topo-p", "topo-sa", "/api/v1/topo-dedupe",
			&spectrev1.AppliedListenerPlacementStatus{Fingerprint: "fp-dedupe", DeliveryZone: ref("zone-obs")})

		// Guard: the fixtures below rely on the applied placement staying put.
		Consistently(func(g Gomega) {
			for _, name := range []string{"topo-applied", "topo-dedupe"} {
				cached := &spectrev1.Listener{}
				g.Expect(k8sClient.Get(ctx, key(name), cached)).To(Succeed())
				g.Expect(cached.Status.AppliedPlacement).NotTo(BeNil(), "applied placement of %s was cleared", name)
			}
		}, 2*time.Second, testInterval).Should(Succeed())
	})

	It("enqueues a Listener when only its observer A's Zone changes", func() {
		Expect(reconciler.mapZoneToListeners(ctx, zoneObj("zone-obs", topoEnv))).To(ContainElement(req("topo-listener")))
	})

	It("enqueues a Listener when only A's EventConfig changes", func() {
		Expect(reconciler.mapEventConfigToListeners(ctx, ecObj("zone-obs", topoEnv))).To(ContainElement(req("topo-listener")))
	})

	It("enqueues a Listener when the backend zone behind A's proxy EventConfig changes", func() {
		Expect(reconciler.mapEventConfigToListeners(ctx, ecObj("zone-backend", topoEnv))).To(ContainElement(req("topo-listener")))
		Expect(reconciler.mapZoneToListeners(ctx, zoneObj("zone-backend", topoEnv))).To(ContainElement(req("topo-listener")))
	})

	It("enqueues a Listener when A's EventStore changes", func() {
		Expect(reconciler.mapEventStoreToListeners(ctx, esObj("es-obs", topoEnv))).To(ContainElement(req("topo-listener")))
	})

	It("enqueues a Listener when the Realm of A's Zone changes", func() {
		zone := &adminv1.Zone{
			ObjectMeta: meta("zone-obs", topoEnv),
			Spec: adminv1.ZoneSpec{
				IdentityProvider: adminv1.IdentityProviderConfig{
					Url:   "http://id.local",
					Admin: adminv1.IdentityProviderAdminConfig{ClientId: "a", UserName: "a", Password: "a"},
				},
				Gateway: adminv1.GatewayConfig{
					Admin: adminv1.GatewayAdminConfig{Url: "http://gw.local"},
					Presets: []adminv1.GatewayConfigPreset{{
						Name: "default", Default: true,
						Urls: []adminv1.UrlConfig{{Hostname: "gw.topo.local", BasePath: "/"}},
					}},
				},
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}
		Expect(client.IgnoreAlreadyExists(directClient.Create(ctx, zone))).To(Succeed())
		Eventually(func(g Gomega) {
			fetched := &adminv1.Zone{}
			g.Expect(directClient.Get(ctx, key("zone-obs"), fetched)).To(Succeed())
			fetched.Status.IdentityRealm = ref("topo-realm")
			fetched.Status.Namespace = "topo-env--zone-obs"
			fetched.Status.Links = adminv1.Links{
				Url:       "http://gw.topo.local",
				Issuer:    "http://id.local/realms/topo-env",
				LmsIssuer: "http://id.local/realms/topo-env-lms",
			}
			g.Expect(directClient.Status().Update(ctx, fetched)).To(Succeed())
		}, testTimeout, testInterval).Should(Succeed())
		Eventually(func(g Gomega) {
			cached := &adminv1.Zone{}
			g.Expect(k8sClient.Get(ctx, key("zone-obs"), cached)).To(Succeed())
			g.Expect(cached.Status.IdentityRealm).NotTo(BeNil())
		}, testTimeout, testInterval).Should(Succeed())

		realm := &identityv1.Realm{ObjectMeta: meta("topo-realm", topoEnv)}
		Expect(reconciler.mapRealmToListeners(ctx, realm)).To(ContainElement(req("topo-listener")))
	})

	DescribeTable("enqueues a Listener through its applied placement ref although its spec matches nothing",
		func(mapFn func() []reconcile.Request) {
			reqs := mapFn()
			Expect(reqs).To(ContainElement(req("topo-applied")))
			Expect(reqs).NotTo(ContainElement(req("topo-listener")))
		},
		Entry("CaptureZone", func() []reconcile.Request {
			return reconciler.mapZoneToListeners(ctx, zoneObj("zone-old-cap", topoEnv))
		}),
		Entry("DeliveryZone", func() []reconcile.Request {
			return reconciler.mapZoneToListeners(ctx, zoneObj("zone-old-del", topoEnv))
		}),
		Entry("CallbackOriginZone", func() []reconcile.Request {
			return reconciler.mapZoneToListeners(ctx, zoneObj("zone-old-origin", topoEnv))
		}),
		Entry("CaptureZone via its EventConfig", func() []reconcile.Request {
			return reconciler.mapEventConfigToListeners(ctx, ecObj("zone-old-cap", topoEnv))
		}),
		// No EventConfig references es-old-cap or es-old-del: only the status can match.
		Entry("CaptureEventStore", func() []reconcile.Request {
			return reconciler.mapEventStoreToListeners(ctx, esObj("es-old-cap", topoEnv))
		}),
		Entry("DeliveryEventStore", func() []reconcile.Request {
			return reconciler.mapEventStoreToListeners(ctx, esObj("es-old-del", topoEnv))
		}),
		// The Route name differs from the Listener's normalized apiBasePath.
		Entry("CaptureRoute", func() []reconcile.Request {
			return reconciler.mapRouteToListeners(ctx, routeObj("route-old", topoEnv))
		}),
	)

	It("still enqueues a Listener whose applied generic Publisher sits in an old zone namespace", func() {
		pub := &pubsubv1.Publisher{ObjectMeta: metav1.ObjectMeta{
			Name:      util.MakePublisherName(util.GenericEventType),
			Namespace: "old-zone-ns",
			Labels:    map[string]string{envLabelKey: topoEnv},
		}}
		Expect(reconciler.mapGenericPublisherToListeners(ctx, pub)).To(ContainElement(req("topo-applied")))
	})

	It("enqueues a Listener once when spec and applied refs hit the same Zone", func() {
		Expect(countOf(reconciler.mapZoneToListeners(ctx, zoneObj("zone-obs", topoEnv)), "topo-dedupe")).To(Equal(1))
		Expect(countOf(reconciler.mapEventConfigToListeners(ctx, ecObj("zone-obs", topoEnv)), "topo-dedupe")).To(Equal(1))
	})

	It("ignores other environments and unlabelled objects", func() {
		for _, env := range []string{topoOther, ""} {
			Expect(reconciler.mapZoneToListeners(ctx, zoneObj("zone-obs", env))).To(BeEmpty())
			Expect(reconciler.mapEventConfigToListeners(ctx, ecObj("zone-obs", env))).To(BeEmpty())
			Expect(reconciler.mapEventStoreToListeners(ctx, esObj("es-old-cap", env))).To(BeEmpty())
			Expect(reconciler.mapRouteToListeners(ctx, routeObj("route-old", env))).To(BeEmpty())
		}
	})

	// A synthetic status.draining on an envtest Listener is consumed by
	// continueDrain within a few reconciles, so drain refs are tested here on
	// in-memory Listeners. The eventStores wiring is proven by the envtest
	// applied-EventStore entries above.
	Describe("listenerTargets.statusRefersTo", func() {
		drainKey := key("es-drain")
		draining := func(es *ctypes.ObjectRef) *spectrev1.Listener {
			return &spectrev1.Listener{Status: spectrev1.ListenerStatus{
				Draining: &spectrev1.ListenerDrainStatus{Phase: handler.DrainPhaseStopping, SourceEventStore: es},
			}}
		}

		It("matches a drain's source EventStore", func() {
			t := &listenerTargets{eventStores: refSet{drainKey: {}}}
			Expect(t.statusRefersTo(draining(ref("es-drain")))).To(BeTrue())
		})

		It("does not match another EventStore or a target of another kind with the same key", func() {
			other := &listenerTargets{eventStores: refSet{key("es-other"): {}}}
			Expect(other.statusRefersTo(draining(ref("es-drain")))).To(BeFalse())

			mixed := &listenerTargets{apps: refSet{drainKey: {}}, zones: refSet{drainKey: {}}, routes: refSet{drainKey: {}}}
			Expect(mixed.statusRefersTo(draining(ref("es-drain")))).To(BeFalse())
			applied := &spectrev1.Listener{Status: spectrev1.ListenerStatus{
				AppliedPlacement: &spectrev1.AppliedListenerPlacementStatus{CaptureZone: ref("es-drain")},
			}}
			Expect((&listenerTargets{eventStores: refSet{drainKey: {}}}).statusRefersTo(applied)).To(BeFalse())
		})

		It("is false for nil placement, nil drain, nil refs and nil target sets", func() {
			all := &listenerTargets{
				apps: refSet{drainKey: {}}, zones: refSet{drainKey: {}},
				eventStores: refSet{drainKey: {}}, routes: refSet{drainKey: {}},
			}
			Expect(all.statusRefersTo(&spectrev1.Listener{})).To(BeFalse())
			Expect(all.statusRefersTo(&spectrev1.Listener{Status: spectrev1.ListenerStatus{
				AppliedPlacement: &spectrev1.AppliedListenerPlacementStatus{Fingerprint: "fp"},
				Draining:         &spectrev1.ListenerDrainStatus{Phase: handler.DrainPhaseStopping},
			}})).To(BeFalse())
			Expect((&listenerTargets{}).statusRefersTo(draining(ref("es-drain")))).To(BeFalse())
		})
	})
})
