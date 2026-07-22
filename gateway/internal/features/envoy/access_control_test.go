// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"

	rbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	jwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	httprbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

// filterByName finds an HTTP filter by its canonical name.
func filterByName(filters []*hcmv3.HttpFilter, name string) *hcmv3.HttpFilter {
	for _, f := range filters {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

func pathKeys(md *matcherv3.MetadataMatcher) []string {
	keys := make([]string, 0, len(md.GetPath()))
	for _, seg := range md.GetPath() {
		keys = append(keys, seg.GetKey())
	}
	return keys
}

var _ = Describe("buildAccessControlFilters", func() {

	Context("with trusted issuers and a consumer allow-list", func() {
		It("emits jwt_authn, rbac, and router in order", func() {
			filters, err := buildAccessControlFilters(accessControlIntent{
				trustedIssuers: []string{"https://iss-a", "https://iss-b"},
				accessControl:  true,
				allowConsumers: []string{"client-a", "client-b"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(HaveLen(3))
			Expect(filters[0].GetName()).To(Equal(filterJwtAuthn))
			Expect(filters[1].GetName()).To(Equal(filterRBAC))
			Expect(filters[2].GetName()).To(Equal(filterRouter))
		})

		It("configures one JWT provider per trusted issuer with azp exported to metadata", func() {
			filters, err := buildAccessControlFilters(accessControlIntent{
				trustedIssuers: []string{"https://iss-a", "https://iss-b"},
			})
			Expect(err).NotTo(HaveOccurred())

			jwt := &jwtauthnv3.JwtAuthentication{}
			Expect(filterByName(filters, filterJwtAuthn).GetTypedConfig().UnmarshalTo(jwt)).To(Succeed())

			Expect(jwt.GetProviders()).To(HaveLen(2))
			issuers := []string{}
			for _, p := range jwt.GetProviders() {
				issuers = append(issuers, p.GetIssuer())
				Expect(p.GetPayloadInMetadata()).To(Equal(jwtPayloadMetadataKey))
				remote := p.GetRemoteJwks()
				Expect(remote).NotTo(BeNil())
				Expect(remote.GetHttpUri().GetUri()).To(Equal(jwksURIFromIssuer(p.GetIssuer())))
				Expect(remote.GetHttpUri().GetCluster()).To(Equal(jwksClusterName(issuerHost(p.GetIssuer()))))
			}
			Expect(issuers).To(ConsistOf("https://iss-a", "https://iss-b"))
		})

		It("matches each allowed consumer against azp via jwt metadata under an ALLOW policy", func() {
			filters, err := buildAccessControlFilters(accessControlIntent{
				trustedIssuers: []string{"https://iss-a"},
				accessControl:  true,
				allowConsumers: []string{"client-a", "client-b"},
			})
			Expect(err).NotTo(HaveOccurred())

			rbac := &httprbacv3.RBAC{}
			Expect(filterByName(filters, filterRBAC).GetTypedConfig().UnmarshalTo(rbac)).To(Succeed())

			Expect(rbac.GetRules().GetAction()).To(Equal(rbacv3.RBAC_ALLOW))
			Expect(rbac.GetRules().GetPolicies()).To(HaveLen(1))

			policy := rbac.GetRules().GetPolicies()["consumer-allowlist"]
			Expect(policy).NotTo(BeNil())
			Expect(policy.GetPrincipals()).To(HaveLen(2))

			matched := []string{}
			for _, pr := range policy.GetPrincipals() {
				md := pr.GetMetadata()
				Expect(md.GetFilter()).To(Equal(filterJwtAuthn))
				Expect(pathKeys(md)).To(Equal([]string{jwtPayloadMetadataKey, consumerMatchClaim}))
				matched = append(matched, md.GetValue().GetStringMatch().GetExact())
			}
			Expect(matched).To(ConsistOf("client-a", "client-b"))
		})
	})

	Context("when access control is disabled", func() {
		It("omits the rbac filter but keeps jwt_authn", func() {
			filters, err := buildAccessControlFilters(accessControlIntent{
				trustedIssuers: []string{"https://iss-a"},
				accessControl:  false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(filterByName(filters, filterRBAC)).To(BeNil())
			Expect(filterByName(filters, filterJwtAuthn)).NotTo(BeNil())
			Expect(filterByName(filters, filterRouter)).NotTo(BeNil())
		})
	})

	Context("with an empty allow-list (deny-all / DenyAllGroup equivalent)", func() {
		It("emits an ALLOW rbac with no policies, denying all requests", func() {
			filters, err := buildAccessControlFilters(accessControlIntent{
				trustedIssuers: []string{"https://iss-a"},
				accessControl:  true,
				allowConsumers: []string{},
			})
			Expect(err).NotTo(HaveOccurred())

			rbac := &httprbacv3.RBAC{}
			Expect(filterByName(filters, filterRBAC).GetTypedConfig().UnmarshalTo(rbac)).To(Succeed())

			Expect(rbac.GetRules().GetAction()).To(Equal(rbacv3.RBAC_ALLOW))
			// ALLOW + zero policies => nothing matches => deny-all.
			Expect(rbac.GetRules().GetPolicies()).To(BeEmpty())
		})
	})
})

var _ = Describe("AccessControlFeature", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	newRoute := func() *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{
				Security: gatewayv1.Security{
					TrustedIssuers:   []string{"https://iss-a"},
					DefaultConsumers: []string{"default-consumer"},
				},
			},
		}
	}

	It("is used only when the route has trusted issuers", func() {
		b := NewFeatureBuilder(nil, newRoute(), nil, nil)
		Expect(InstanceAccessControlFeature.IsUsed(ctx, b)).To(BeTrue())

		noIss := newRoute()
		noIss.Spec.Security.TrustedIssuers = nil
		b2 := NewFeatureBuilder(nil, noIss, nil, nil)
		Expect(InstanceAccessControlFeature.IsUsed(ctx, b2)).To(BeFalse())
	})

	It("resolves the allow-list from default + route-matched consumers", func() {
		route := newRoute()
		matching := &gatewayv1.ConsumeRoute{
			Spec: gatewayv1.ConsumeRouteSpec{
				ConsumerName: "matching-consumer",
				Route:        ctypes.ObjectRef{Name: route.Name, Namespace: route.Namespace},
			},
		}
		other := &gatewayv1.ConsumeRoute{
			Spec: gatewayv1.ConsumeRouteSpec{
				ConsumerName: "other-consumer",
				Route:        ctypes.ObjectRef{Name: "different-route", Namespace: route.Namespace},
			},
		}
		b := NewFeatureBuilder(nil, route, nil, nil).(*Builder)
		b.AddAllowedConsumers(matching, other)

		Expect(InstanceAccessControlFeature.Apply(ctx, b)).To(Succeed())
		Expect(b.intent.trustedIssuers).To(ConsistOf("https://iss-a"))
		Expect(b.intent.accessControl).To(BeTrue())
		Expect(b.intent.allowConsumers).To(ConsistOf("default-consumer", "matching-consumer"))
	})

	It("leaves access control off (allowConsumers nil) when disabled", func() {
		route := newRoute()
		route.Spec.Security.DisableAccessControl = true
		b := NewFeatureBuilder(nil, route, nil, nil).(*Builder)

		Expect(InstanceAccessControlFeature.Apply(ctx, b)).To(Succeed())
		Expect(b.intent.trustedIssuers).To(ConsistOf("https://iss-a"))
		Expect(b.intent.accessControl).To(BeFalse())
		Expect(b.intent.allowConsumers).To(BeNil())
	})
})

var _ = Describe("Builder.Build", func() {
	var (
		ctx   context.Context
		cache cachev3.SnapshotCache
		xds   XdsClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		cache = cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		xds = NewXdsClient(cache)
	})

	newRoute := func() *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{
				Security: gatewayv1.Security{
					TrustedIssuers:   []string{"https://iss-a"},
					DefaultConsumers: []string{"default-consumer"},
				},
			},
		}
	}

	It("runs the feature and publishes a consistent snapshot that round-trips", func() {
		b := NewFeatureBuilder(xds, newRoute(), nil, nil)
		b.EnableFeature(InstanceAccessControlFeature)
		b.SetUpstream(client.NewUpstreamOrDie("https://backend.svc.local:8080/api"))

		Expect(b.Build(ctx)).To(Succeed())

		snap, err := cache.GetSnapshot(PocNodeID)
		Expect(err).NotTo(HaveOccurred())
		// SetSnapshotFor already asserts Consistent() before publishing; confirm
		// the published snapshot carries the expected resource kinds.
		Expect(snap.GetResources(resource.ListenerType)).To(HaveLen(1))
		// upstream cluster + one jwks cluster for the issuer host.
		Expect(snap.GetResources(resource.ClusterType)).To(HaveLen(2))
		Expect(snap.GetResources(resource.ClusterType)).To(HaveKey(jwksClusterName("iss-a")))
	})

	It("returns an error when upstream is not set", func() {
		b := NewFeatureBuilder(xds, newRoute(), nil, nil)
		b.EnableFeature(InstanceAccessControlFeature)
		Expect(b.Build(ctx)).To(MatchError(ContainSubstring("upstream")))
	})
})
