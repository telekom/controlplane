// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/index"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func proxyVoyagerRouteNames(obj *eventv1.EventConfig) []string {
	names := make([]string, 0, len(obj.Status.ProxyVoyagerRoutes))
	for _, ref := range obj.Status.ProxyVoyagerRoutes {
		names = append(names, ref.Name)
	}
	return names
}

var _ = Describe("voyager route status ordering", func() {
	var (
		ctx   context.Context
		fc    *fakeclient.MockJanitorClient
		zones map[string]*adminv1.Zone
	)

	peerNames := []string{"z-zone", "m-zone", "a-zone"}

	BeforeEach(func() {
		ctx = context.Background()
		fc = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fc)
		zones = map[string]*adminv1.Zone{}
		for _, name := range append([]string{"source", "backend"}, peerNames...) {
			zone := readyPeerZone(name)
			withEventPreset(zone, name+"-gateway", name+".example.com", &adminv1.Links{Issuer: "https://" + name + "-idp", LmsIssuer: "https://" + name + "-lms"})
			zones[name] = zone
		}
		fc.EXPECT().Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Zone")).
			RunAndReturn(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) error {
				zone, ok := zones[key.Name]
				Expect(ok).To(BeTrue(), "unexpected zone %q", key.Name)
				*out.(*adminv1.Zone) = *zone
				return nil
			}).Maybe()
		fc.EXPECT().Scheme().Return(buildSchemeForCallbackRoutes()).Maybe()
		fc.EXPECT().CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				return controllerutil.OperationResultCreated, mutate()
			}).Maybe()
	})

	peers := func(extra ...eventv1.EventConfig) []eventv1.EventConfig {
		items := append([]eventv1.EventConfig{}, extra...)
		for _, name := range peerNames {
			items = append(items, peerEventConfig(name+"-config", name, nil, nil))
		}
		return items
	}

	expectListPeers := func(items []eventv1.EventConfig) {
		fc.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: items}
			}).Return(nil).Once()
	}

	It("sorts local outbound Voyager route references across multiple peers", func() {
		obj := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "source-config", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				Zone:  ctypes.ObjectRef{Name: "source", Namespace: "default"},
				Local: &eventv1.LocalBackend{VoyagerApiUrl: "http://voyager.local"},
				Mesh:  &eventv1.MeshConfig{FullMesh: true},
			},
		}
		expectListPeers(peers())

		Expect((&EventConfigHandler{}).createVoyagerRoutes(ctx, obj, zones["source"], obj.Spec.Mesh)).To(Succeed())

		Expect(proxyVoyagerRouteNames(obj)).To(Equal([]string{"voyager--a-zone", "voyager--m-zone", "voyager--z-zone"}))
		Expect(obj.Status.ProxyVoyagerURLs).To(HaveLen(3))
		Expect(obj.Status.ProxyVoyagerURLs).To(SatisfyAll(HaveKey("a-zone"), HaveKey("m-zone"), HaveKey("z-zone")))
	})

	It("sorts proxy outbound Voyager route references across multiple peers", func() {
		backend := peerEventConfig("backend-config", "backend", nil, nil)
		backend.Spec.Local = &eventv1.LocalBackend{VoyagerApiUrl: "http://voyager.backend"}
		meta.SetStatusCondition(&backend.Status.Conditions, metav1.Condition{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready"})
		obj := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "source-config", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				Zone:  ctypes.ObjectRef{Name: "source", Namespace: "default"},
				Proxy: &eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "backend", Namespace: "default"}},
				Mesh:  &eventv1.MeshConfig{FullMesh: true},
			},
		}
		fc.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventConfigList"), client.MatchingFields{
			index.EventConfigZoneIndex: "backend",
		}).Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{backend}}
		}).Return(nil).Once()
		expectListPeers(peers(backend))

		Expect((&EventConfigHandler{}).createProxyVoyagerRoutes(ctx, obj, zones["source"], obj.Spec.Mesh)).To(Succeed())

		Expect(proxyVoyagerRouteNames(obj)).To(Equal([]string{"voyager--a-zone", "voyager--backend", "voyager--m-zone", "voyager--z-zone"}))
		Expect(obj.Status.ProxyVoyagerURLs).To(HaveLen(4))
	})
	It("records Voyager status deterministically regardless of map iteration order", func() {
		routes := map[string]*gatewayv1.Route{}
		for _, zone := range []string{"z-zone", "a-zone", "m-zone", "b-zone", "y-zone"} {
			routes[zone] = &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "voyager--" + zone, Namespace: "default"}}
		}
		first := &eventv1.EventConfig{Status: eventv1.EventConfigStatus{
			ProxyVoyagerRoutes: []ctypes.ObjectRef{{Name: "voyager--stale", Namespace: "default"}},
			ProxyVoyagerURLs:   map[string]string{"stale": "https://stale"},
		}}
		setProxyVoyagerStatus(first, routes)

		Expect(proxyVoyagerRouteNames(first)).To(Equal([]string{
			"voyager--a-zone", "voyager--b-zone", "voyager--m-zone", "voyager--y-zone", "voyager--z-zone",
		}))
		Expect(first.Status.ProxyVoyagerURLs).To(HaveLen(5))
		Expect(first.Status.ProxyVoyagerURLs).NotTo(HaveKey("stale"))

		for range 20 {
			again := &eventv1.EventConfig{}
			setProxyVoyagerStatus(again, routes)
			Expect(again.Status).To(Equal(first.Status))
		}
	})

	It("records empty Voyager status when there are no routes", func() {
		obj := &eventv1.EventConfig{}
		setProxyVoyagerStatus(obj, nil)
		Expect(obj.Status.ProxyVoyagerRoutes).To(BeEmpty())
		Expect(obj.Status.ProxyVoyagerURLs).To(BeEmpty())
	})
})
