// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
// Copyright 2026.
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
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
