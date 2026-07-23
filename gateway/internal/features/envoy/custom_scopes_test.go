// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"encoding/json"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	extauthzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

var _ = Describe("CustomScopesFeature", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	newRoute := func() *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec:       gatewayv1.RouteSpec{Type: gatewayv1.RouteTypePrimary, Security: gatewayv1.Security{}},
		}
	}

	Context("IsUsed", func() {
		It("is used for a primary non-pass-through route", func() {
			b := NewFeatureBuilder(nil, newRoute(), nil, nil)
			Expect(InstanceCustomScopesFeature.IsUsed(ctx, b)).To(BeTrue())
		})

		It("is used for a failover-secondary route", func() {
			r := newRoute()
			r.Spec.Type = gatewayv1.RouteTypeSecondary
			b := NewFeatureBuilder(nil, r, nil, nil)
			Expect(InstanceCustomScopesFeature.IsUsed(ctx, b)).To(BeTrue())
		})

		It("is not used for a proxy route", func() {
			r := newRoute()
			r.Spec.Type = gatewayv1.RouteTypeProxy
			b := NewFeatureBuilder(nil, r, nil, nil)
			Expect(InstanceCustomScopesFeature.IsUsed(ctx, b)).To(BeFalse())
		})

		It("is not used for a pass-through route", func() {
			r := newRoute()
			r.Spec.PassThrough = true
			b := NewFeatureBuilder(nil, r, nil, nil)
			Expect(InstanceCustomScopesFeature.IsUsed(ctx, b)).To(BeFalse())
		})

		It("is not used with no route", func() {
			b := NewFeatureBuilder(nil, nil, nil, nil)
			Expect(InstanceCustomScopesFeature.IsUsed(ctx, b)).To(BeFalse())
		})
	})

	Context("Apply", func() {
		It("returns ErrNoRoute when no route is set", func() {
			b := NewFeatureBuilder(nil, nil, nil, nil).(*Builder)
			Expect(InstanceCustomScopesFeature.Apply(ctx, b)).To(MatchError(features.ErrNoRoute))
		})
		It("declares the route default scopes (space-joined)", func() {
			r := newRoute()
			r.Spec.Security.M2M = &gatewayv1.Machine2MachineAuthentication{
				Scopes: []string{"read", "write", "admin"},
			}
			b := NewFeatureBuilder(nil, r, nil, nil).(*Builder)

			Expect(InstanceCustomScopesFeature.Apply(ctx, b)).To(Succeed())
			Expect(b.customScopes.defaultScopes).To(Equal("read write admin"))
			Expect(b.customScopes.perConsumer).To(BeEmpty())
		})

		It("declares per-consumer scopes keyed by consumer name (space-joined)", func() {
			b := NewFeatureBuilder(nil, newRoute(), nil, nil).(*Builder)
			b.AddAllowedConsumers(
				&gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
					ConsumerName: "foo",
					Security: &gatewayv1.ConsumeRouteSecurity{
						M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{Scopes: []string{"read"}},
					},
				}},
				&gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
					ConsumerName: "bar",
					Security: &gatewayv1.ConsumeRouteSecurity{
						M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{Scopes: []string{"write", "admin"}},
					},
				}},
			)

			Expect(InstanceCustomScopesFeature.Apply(ctx, b)).To(Succeed())
			Expect(b.customScopes.perConsumer).To(HaveKeyWithValue("foo", "read"))
			Expect(b.customScopes.perConsumer).To(HaveKeyWithValue("bar", "write admin"))
		})

		It("ignores consumers without M2M scopes", func() {
			b := NewFeatureBuilder(nil, newRoute(), nil, nil).(*Builder)
			b.AddAllowedConsumers(
				&gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
					ConsumerName: "no-m2m",
					Security:     &gatewayv1.ConsumeRouteSecurity{M2M: nil},
				}},
			)

			Expect(InstanceCustomScopesFeature.Apply(ctx, b)).To(Succeed())
			Expect(b.customScopes.perConsumer).To(BeEmpty())
			Expect(b.customScopes.defaultScopes).To(BeEmpty())
		})
	})
})

var _ = Describe("lmsVhostPerFilterConfig with custom scopes", func() {
	It("does not emit scopes when LMS is disabled", func() {
		m, err := lmsVhostPerFilterConfig(
			lmsIntent{enabled: false},
			customScopesIntent{defaultScopes: "read", perConsumer: map[string]string{"foo": "read"}},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(m).To(BeNil())
	})

	It("folds default and per-consumer scopes into ext_authz context_extensions", func() {
		m, err := lmsVhostPerFilterConfig(
			lmsIntent{enabled: true, realm: "r1", environment: "poc"},
			customScopesIntent{
				defaultScopes: "read",
				perConsumer:   map[string]string{"foo": "read", "bar": "write admin"},
			},
		)
		Expect(err).NotTo(HaveOccurred())

		perRoute := &extauthzv3.ExtAuthzPerRoute{}
		Expect(m[filterExtAuthz].UnmarshalTo(perRoute)).To(Succeed())
		ext := perRoute.GetCheckSettings().GetContextExtensions()

		// realm/environment preserved alongside the scopes.
		Expect(ext).To(HaveKeyWithValue("realm", "r1"))
		Expect(ext).To(HaveKeyWithValue("environment", "poc"))
		Expect(ext).To(HaveKeyWithValue("defaultScopes", "read"))

		// per-consumer map is a JSON object in a single opaque value.
		Expect(ext).To(HaveKey("consumerScopes"))
		decoded := map[string]string{}
		Expect(json.Unmarshal([]byte(ext["consumerScopes"]), &decoded)).To(Succeed())
		Expect(decoded).To(HaveKeyWithValue("foo", "read"))
		Expect(decoded).To(HaveKeyWithValue("bar", "write admin"))
	})

	It("omits scope keys when no scopes are declared", func() {
		m, err := lmsVhostPerFilterConfig(lmsIntent{enabled: true, realm: "r1", environment: "poc"}, customScopesIntent{})
		Expect(err).NotTo(HaveOccurred())

		perRoute := &extauthzv3.ExtAuthzPerRoute{}
		Expect(m[filterExtAuthz].UnmarshalTo(perRoute)).To(Succeed())
		ext := perRoute.GetCheckSettings().GetContextExtensions()
		Expect(ext).NotTo(HaveKey("defaultScopes"))
		Expect(ext).NotTo(HaveKey("consumerScopes"))
	})
})

var _ = Describe("CustomScopes end-to-end snapshot", func() {
	It("publishes the scopes in the vhost ext_authz context_extensions", func() {
		ctx := contextutil.WithEnv(context.Background(), "poc")

		route := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{
				Type:     gatewayv1.RouteTypePrimary,
				Security: gatewayv1.Security{RealmName: "poc-realm"},
			},
		}
		route.Spec.Security.M2M = &gatewayv1.Machine2MachineAuthentication{Scopes: []string{"read"}}

		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		xds := NewXdsClient(cache)
		b := NewFeatureBuilder(xds, route, nil, nil)
		b.EnableFeature(InstanceLastMileSecurityFeature)
		b.EnableFeature(InstanceCustomScopesFeature)
		b.AddAllowedConsumers(&gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
			ConsumerName: "foo",
			Security: &gatewayv1.ConsumeRouteSecurity{
				M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{Scopes: []string{"write"}},
			},
		}})
		b.SetUpstream(client.NewUpstreamOrDie("https://backend.svc.local:8080/api"))

		Expect(b.Build(ctx)).To(Succeed())

		snap, err := cache.GetSnapshot(PocNodeID)
		Expect(err).NotTo(HaveOccurred())

		listeners := snap.GetResources(resource.ListenerType)
		Expect(listeners).To(HaveKey(route.Name))
		lst := listeners[route.Name].(*listenerv3.Listener)

		// Unwrap HCM -> RouteConfig -> VirtualHost -> ext_authz per-filter config.
		hcm := &hcmv3.HttpConnectionManager{}
		Expect(lst.GetFilterChains()[0].GetFilters()[0].GetTypedConfig().UnmarshalTo(hcm)).To(Succeed())
		vhost := hcm.GetRouteConfig().GetVirtualHosts()[0]

		perRoute := &extauthzv3.ExtAuthzPerRoute{}
		Expect(vhost.GetTypedPerFilterConfig()[filterExtAuthz].UnmarshalTo(perRoute)).To(Succeed())
		ext := perRoute.GetCheckSettings().GetContextExtensions()

		Expect(ext).To(HaveKeyWithValue("defaultScopes", "read"))
		decoded := map[string]string{}
		Expect(json.Unmarshal([]byte(ext["consumerScopes"]), &decoded)).To(Succeed())
		Expect(decoded).To(HaveKeyWithValue("foo", "write"))
	})
})
