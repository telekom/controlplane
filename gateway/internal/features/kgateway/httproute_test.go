// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"

	kgwv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("KGateway render", func() {
	newBuilder := func(route *gatewayv1.Route) *Builder {
		gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "my-gateway", Namespace: "ns"}}
		b := NewFeatureBuilder(nil, route, nil, gw).(*Builder)
		b.SetUpstream(client.NewUpstreamOrDie("https://backend.svc.local:8080/api"))
		return b
	}

	It("renders a Backend from the selected upstream", func() {
		route := &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"}}
		be := newBuilder(route).buildBackend()

		Expect(be.Name).To(Equal("my-route"))
		Expect(be.Namespace).To(Equal("ns"))
		Expect(be.Spec.Static).NotTo(BeNil())
		Expect(be.Spec.Static.Hosts).To(HaveLen(1))
		Expect(be.Spec.Static.Hosts[0].Host).To(Equal("backend.svc.local"))
		Expect(be.Spec.Static.Hosts[0].Port).To(Equal(gwapiv1.PortNumber(8080)))
	})

	It("binds the HTTPRoute to the gateway and the backend", func() {
		route := &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"}}
		hr := newBuilder(route).buildHTTPRoute("my-route", nil)

		Expect(hr.Spec.ParentRefs).To(HaveLen(1))
		Expect(string(hr.Spec.ParentRefs[0].Name)).To(Equal("my-gateway"))

		Expect(hr.Spec.Rules).To(HaveLen(1))
		refs := hr.Spec.Rules[0].BackendRefs
		Expect(refs).To(HaveLen(1))
		Expect(string(refs[0].Name)).To(Equal("my-route"))
		Expect(string(*refs[0].Kind)).To(Equal("Backend"))
		Expect(string(*refs[0].Group)).To(Equal(kgwv1alpha1.GroupName))
	})

	It("defaults to a single '/' PathPrefix match when no paths are set", func() {
		route := &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"}}
		hr := newBuilder(route).buildHTTPRoute("my-route", nil)

		matches := hr.Spec.Rules[0].Matches
		Expect(matches).To(HaveLen(1))
		Expect(*matches[0].Path.Type).To(Equal(gwapiv1.PathMatchPathPrefix))
		Expect(*matches[0].Path.Value).To(Equal("/"))
		Expect(hr.Spec.Hostnames).To(BeEmpty())
	})

	It("maps configured paths and hostnames onto the HTTPRoute", func() {
		route := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{
				Paths:     []string{"/foo", "/bar"},
				Hostnames: []string{"api.example.com"},
			},
		}
		hr := newBuilder(route).buildHTTPRoute("my-route", nil)

		Expect(hr.Spec.Rules[0].Matches).To(HaveLen(2))
		Expect(*hr.Spec.Rules[0].Matches[0].Path.Value).To(Equal("/foo"))
		Expect(*hr.Spec.Rules[0].Matches[1].Path.Value).To(Equal("/bar"))
		Expect(hr.Spec.Hostnames).To(ConsistOf(gwapiv1.Hostname("api.example.com")))
	})
})
