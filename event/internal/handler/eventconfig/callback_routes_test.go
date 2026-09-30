// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	"github.com/telekom/controlplane/event/internal/index"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// withEventPreset gives the zone an Event preset with the given gateway, hostname
// and links. An empty gateway name leaves the preset status without a GatewayRef.
func withEventPreset(zone *adminv1.Zone, gateway, hostname string, links adminv1.Links) {
	zone.Spec.Presets = []adminv1.Preset{{
		Name: "event", Type: adminv1.GatewayTypeEvent, Default: true,
		Urls: []adminv1.UrlConfig{{Scheme: "https", Hostname: hostname, Port: 443}},
	}}
	status := adminv1.PresetStatus{Name: "event", Links: links}
	if gateway != "" {
		status.GatewayRef = &ctypes.ObjectRef{Name: gateway, Namespace: "default"}
	}
	zone.Status.Presets = []adminv1.PresetStatus{status}
}

var _ = Describe("callback route issuers", func() {
	It("sorts outbound callback route references by name", func() {
		ctx := context.Background()
		fc := fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fc)
		obj := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "source-config", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				Zone: ctypes.ObjectRef{Name: "source", Namespace: "default"},
				Mesh: &eventv1.MeshConfig{FullMesh: true},
			},
		}
		source := readyPeerZone("source")
		withEventPreset(source, "source-gateway", "source.example.com", adminv1.Links{})
		peers := []eventv1.EventConfig{
			peerEventConfig("z-config", "z-zone", nil, nil),
			peerEventConfig("a-config", "a-zone", nil, nil),
		}
		fc.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: peers}
			}).Return(nil).Once()
		for _, name := range []string{"z-zone", "a-zone"} {
			zone := readyPeerZone(name)
			withEventPreset(zone, name+"-gateway", name+".example.com", adminv1.Links{})
			fc.EXPECT().Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: "default"}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).Return(nil).Once()
		}
		fc.EXPECT().Scheme().Return(buildSchemeForCallbackRoutes()).Maybe()
		fc.EXPECT().CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				return controllerutil.OperationResultCreated, mutate()
			}).Times(3)

		Expect((&EventConfigHandler{}).createCallbackRoutes(ctx, obj, source, obj.Spec.Mesh, util.CallbackClientName)).To(Succeed())
		Expect(obj.Status.ProxyCallbackRoutes).To(HaveLen(2))
		Expect(obj.Status.ProxyCallbackRoutes[0].Name).To(Equal("callback--a-zone"))
		Expect(obj.Status.ProxyCallbackRoutes[1].Name).To(Equal("callback--z-zone"))
	})

	It("replaces stale projected URLs when the backend or logical mesh changes", func() {
		proxy := peerEventConfig("proxy", "exposure", &eventv1.MeshConfig{FullMesh: true}, &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "old"}})
		oldBackend := peerEventConfig("old", "old", &eventv1.MeshConfig{FullMesh: true}, nil)
		oldBackend.Status.ProxyCallbackURLs = map[string]string{"exposure": "https://old/exposure", "subscriber": "https://old/subscriber"}
		proxy.Status.CallbackURL, proxy.Status.ProxyCallbackURLs = projectCallbackIngress(&proxy, &oldBackend, []string{"subscriber"})
		Expect(proxy.Status.ProxyCallbackURLs).To(HaveKey("subscriber"))

		proxy.Spec.Proxy.TargetZone.Name = "new"
		proxy.Spec.Mesh = &eventv1.MeshConfig{ZoneNames: []string{"new"}}
		newBackend := peerEventConfig("new", "new", &eventv1.MeshConfig{ZoneNames: []string{"exposure"}}, nil)
		newBackend.Status.CallbackURL = "https://new/primary"
		newBackend.Status.ProxyCallbackURLs = map[string]string{"exposure": "https://new/exposure"}
		proxy.Status.CallbackURL, proxy.Status.ProxyCallbackURLs = projectCallbackIngress(&proxy, &newBackend, []string{"new"})
		Expect(proxy.Status.CallbackURL).To(Equal("https://new/exposure"))
		Expect(proxy.Status.ProxyCallbackURLs).To(Equal(map[string]string{"new": "https://new/primary"}))
	})

	DescribeTable("projects only reachable backend ingress for the proxy's logical mesh",
		func(mesh, backendMesh *eventv1.MeshConfig, backendURLs, expected map[string]string, primary string) {
			proxy := peerEventConfig("proxy", "exposure", mesh, &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "backend"}})
			backend := peerEventConfig("backend", "backend", backendMesh, nil)
			backend.Status.CallbackURL = "https://backend/primary"
			backend.Status.ProxyCallbackURLs = backendURLs
			url, projected := projectCallbackIngress(&proxy, &backend, []string{"backend", "subscriber", "excluded"})
			Expect(url).To(Equal(primary))
			Expect(projected).To(Equal(expected))
		},
		Entry("rebases self, backend primary and distinct subscriber", &eventv1.MeshConfig{ZoneNames: []string{"backend", "subscriber"}},
			&eventv1.MeshConfig{ZoneNames: []string{"exposure", "subscriber"}}, map[string]string{"exposure": "https://backend/exposure", "subscriber": "https://backend/subscriber", "excluded": "https://backend/excluded"},
			map[string]string{"backend": "https://backend/primary", "subscriber": "https://backend/subscriber"}, "https://backend/exposure"),
		Entry("backend mesh excludes subscriber despite existing URL", &eventv1.MeshConfig{ZoneNames: []string{"subscriber"}},
			&eventv1.MeshConfig{ZoneNames: []string{"exposure"}}, map[string]string{"exposure": "https://backend/exposure", "subscriber": "https://backend/subscriber"}, map[string]string{}, "https://backend/exposure"),
		Entry("missing or empty backend forward cannot fall back to local ingress", &eventv1.MeshConfig{ZoneNames: []string{"subscriber"}},
			&eventv1.MeshConfig{FullMesh: true}, map[string]string{"exposure": "", "subscriber": ""}, map[string]string{}, ""),
		Entry("backend no longer permits the proxy's zone", &eventv1.MeshConfig{ZoneNames: []string{"backend"}},
			&eventv1.MeshConfig{ZoneNames: []string{"subscriber"}}, map[string]string{"exposure": "https://backend/stale"},
			map[string]string{"backend": "https://backend/primary"}, ""),
		Entry("nil mesh allows all known peers", nil, nil,
			map[string]string{"exposure": "https://backend/exposure", "subscriber": "https://backend/subscriber"},
			map[string]string{"backend": "https://backend/primary", "subscriber": "https://backend/subscriber"}, "https://backend/exposure"),
		Entry("full mesh allows all known peers", &eventv1.MeshConfig{FullMesh: true}, &eventv1.MeshConfig{FullMesh: true},
			map[string]string{"exposure": "https://backend/exposure", "subscriber": "https://backend/subscriber"},
			map[string]string{"backend": "https://backend/primary", "subscriber": "https://backend/subscriber"}, "https://backend/exposure"),
		Entry("explicit empty mesh excludes peers", &eventv1.MeshConfig{}, nil,
			map[string]string{"exposure": "https://backend/exposure", "subscriber": "https://backend/subscriber"},
			map[string]string{}, "https://backend/exposure"),
	)

	DescribeTable("authenticates eventstore at the backend ingress and gateway at the subscriber primary",
		func(proxyExposure, proxySubscriber bool) {
			ctx := context.Background()
			fc := fakeclient.NewMockJanitorClient(GinkgoT())
			ctx = cclient.WithClient(ctx, fc)
			obj := &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "exposure-config", Namespace: "default"},
				Spec: eventv1.EventConfigSpec{
					Zone: ctypes.ObjectRef{Name: "exposure", Namespace: "default"},
					Mesh: &eventv1.MeshConfig{ZoneNames: []string{"subscriber"}},
				},
			}
			if proxyExposure {
				obj.Spec.Proxy = &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "backend", Namespace: "default"}}
			}
			exposure := readyPeerZone("exposure")
			withEventPreset(exposure, "exposure-gateway", "exposure.example.com", adminv1.Links{Issuer: "https://exposure-idp", LmsIssuer: "https://exposure-lms"})
			subscriber := readyPeerZone("subscriber")
			withEventPreset(subscriber, "subscriber-gateway", "subscriber.example.com", adminv1.Links{Issuer: "https://subscriber-idp", LmsIssuer: "https://subscriber-lms"})
			var subscriberProxy *eventv1.ProxyBackend
			if proxySubscriber {
				subscriberProxy = &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "backend", Namespace: "default"}}
			}
			peer := peerEventConfig("subscriber-config", "subscriber", &eventv1.MeshConfig{ZoneNames: []string{"backend"}}, subscriberProxy)
			fc.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{peer}}
				}).Return(nil).Once()
			fc.EXPECT().Get(ctx, k8stypes.NamespacedName{Name: "subscriber", Namespace: "default"}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *subscriber
				}).Return(nil).Once()
			if proxyExposure {
				backend := peerEventConfig("backend-config", "backend", &eventv1.MeshConfig{FullMesh: true}, nil)
				backend.Spec.Local = &eventv1.LocalBackend{}
				meta.SetStatusCondition(&backend.Status.Conditions, metav1.Condition{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready"})
				backend.Status.CallbackURL = "https://backend.example.com/horizon-backend/callback/v1"
				backend.Status.ProxyCallbackURLs = map[string]string{
					"exposure":   "https://backend.example.com/horizon-exposure/callback/v1",
					"subscriber": "https://backend.example.com/horizon-subscriber/callback/v1",
				}
				fc.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList"), client.MatchingFields{
					index.EventConfigZoneIndex: "backend",
				}).Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{backend}}
				}).Return(nil).Once()
			}
			fc.EXPECT().Scheme().Return(buildSchemeForCallbackRoutes()).Maybe()
			routeCount := 2
			if proxyExposure {
				routeCount = 1
			}
			fc.EXPECT().CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				RunAndReturn(func(_ context.Context, routeObj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
					Expect(mutate()).To(Succeed())
					route := routeObj.(*gatewayv1.Route)
					if route.Spec.Type == gatewayv1.RouteTypeProxy {
						Expect(proxyExposure).To(BeFalse())
						Expect(route.Spec.Security.TrustedIssuers).To(Equal([]string{"https://exposure-idp"}))
						Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("subscriber.example.com"))
						Expect(route.Spec.Security.DefaultConsumers).To(ConsistOf(util.CallbackClientName))
					} else {
						Expect(route.Spec.Security.TrustedIssuers).To(Equal([]string{"https://exposure-idp"}))
						Expect(route.Spec.Security.DefaultConsumers).To(ConsistOf(util.CallbackClientName))
					}
					return controllerutil.OperationResultCreated, nil
				}).Times(routeCount)

			Expect((&EventConfigHandler{}).createCallbackRoutes(ctx, obj, exposure, obj.Spec.Mesh, util.CallbackClientName)).To(Succeed())
			if proxyExposure {
				Expect(obj.Status.ProxyCallbackRoutes).To(BeEmpty())
				Expect(obj.Status.CallbackRoute).NotTo(BeNil())
				Expect(obj.Status.CallbackURL).To(Equal("https://backend.example.com/horizon-exposure/callback/v1"))
				Expect(obj.Status.ProxyCallbackURLs).To(HaveKeyWithValue("subscriber", "https://backend.example.com/horizon-subscriber/callback/v1"))
			} else {
				Expect(obj.Status.ProxyCallbackRoutes).To(HaveLen(1))
				Expect(obj.Status.ProxyCallbackURLs).To(HaveKeyWithValue("subscriber", "https://exposure.example.com/horizon-subscriber/callback/v1"))
			}
		},
		Entry("local exposure and local subscriber", false, false),
		Entry("local exposure and proxy subscriber", false, true),
		Entry("proxy exposure and local subscriber", true, false),
		Entry("proxy exposure and proxy subscriber", true, true),
	)

	DescribeTable("trusts the backend gateway's LMS issuer at the subscriber primary without requiring a reverse mesh", func(proxySubscriber bool) {
		ctx := context.Background()
		fc := fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fc)
		subscriber := readyPeerZone("subscriber")
		withEventPreset(subscriber, "subscriber-gateway", "subscriber.example.com", adminv1.Links{Issuer: "https://subscriber-idp", LmsIssuer: "https://subscriber-lms"})
		backend := readyPeerZone("backend")
		withEventPreset(backend, "", "backend.example.com", adminv1.Links{Issuer: "https://backend-idp", LmsIssuer: "https://backend-lms/spacegate"})
		obj := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "subscriber-config", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				Zone: ctypes.ObjectRef{Name: "subscriber", Namespace: "default"},
				Mesh: &eventv1.MeshConfig{ZoneNames: []string{"unrelated"}},
			},
		}
		if proxySubscriber {
			obj.Spec.Proxy = &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "backend", Namespace: "default"}}
		}
		peer := peerEventConfig("backend-config", "backend", &eventv1.MeshConfig{ZoneNames: []string{"subscriber"}}, nil)
		fc.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{peer}}
			}).Return(nil).Once()
		fc.EXPECT().Get(ctx, k8stypes.NamespacedName{Name: "backend", Namespace: "default"}, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *backend
			}).Return(nil).Once()
		fc.EXPECT().Scheme().Return(buildSchemeForCallbackRoutes()).Maybe()
		fc.EXPECT().CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, routeObj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				Expect(mutate()).To(Succeed())
				route := routeObj.(*gatewayv1.Route)
				Expect(route.Spec.Type).To(Equal(gatewayv1.RouteTypePrimary))
				Expect(route.Spec.Security.TrustedIssuers).To(ConsistOf("https://subscriber-idp", "https://backend-lms/spacegate"))
				Expect(route.Spec.Security.DefaultConsumers).To(ConsistOf(util.CallbackClientName, gatewayv1.GatewayConsumerName))
				return controllerutil.OperationResultCreated, nil
			}).Once()
		if proxySubscriber {
			backendCfg := peerEventConfig("backend-config", "backend", &eventv1.MeshConfig{FullMesh: true}, nil)
			backendCfg.Spec.Local = &eventv1.LocalBackend{}
			meta.SetStatusCondition(&backendCfg.Status.Conditions, metav1.Condition{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready"})
			backendCfg.Status.ProxyCallbackURLs = map[string]string{"subscriber": "https://backend.example.com/horizon-subscriber/callback/v1"}
			fc.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList"), client.MatchingFields{
				index.EventConfigZoneIndex: "backend",
			}).Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{backendCfg}}
			}).Return(nil).Once()
		}
		Expect((&EventConfigHandler{}).createCallbackRoutes(ctx, obj, subscriber, obj.Spec.Mesh, util.CallbackClientName)).To(Succeed())
		Expect(obj.Status.ProxyCallbackURLs).To(BeEmpty())
		if proxySubscriber {
			Expect(obj.Status.ProxyCallbackRoutes).To(BeEmpty())
		}
	},
		Entry("local subscriber", false),
		Entry("proxy subscriber", true),
	)
})

func buildSchemeForCallbackRoutes() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(eventv1.AddToScheme(scheme)).To(Succeed())
	Expect(gatewayv1.AddToScheme(scheme)).To(Succeed())
	return scheme
}
