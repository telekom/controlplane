// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"

	extauthzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	upstreamsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

var _ = Describe("buildFilters (ext_authz ordering)", func() {

	Context("with LMS enabled after access control", func() {
		It("orders jwt_authn -> rbac -> ext_authz -> router", func() {
			filters, err := buildFilters(
				accessControlIntent{
					trustedIssuers: []string{"https://iss-a"},
					accessControl:  true,
					allowConsumers: []string{"client-a"},
				},
				lmsIntent{enabled: true, realm: "r1", environment: "poc"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(HaveLen(4))
			Expect(filters[0].GetName()).To(Equal(filterJwtAuthn))
			Expect(filters[1].GetName()).To(Equal(filterRBAC))
			Expect(filters[2].GetName()).To(Equal(filterExtAuthz))
			Expect(filters[3].GetName()).To(Equal(filterRouter))
		})
	})

	Context("with LMS disabled", func() {
		It("emits no ext_authz filter", func() {
			filters, err := buildFilters(
				accessControlIntent{trustedIssuers: []string{"https://iss-a"}},
				lmsIntent{enabled: false},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(filterByName(filters, filterExtAuthz)).To(BeNil())
			// router still terminal.
			Expect(filters[len(filters)-1].GetName()).To(Equal(filterRouter))
		})
	})

	It("points ext_authz at the issuer gRPC cluster and fails closed", func() {
		filters, err := buildFilters(accessControlIntent{}, lmsIntent{enabled: true})
		Expect(err).NotTo(HaveOccurred())

		cfg := &extauthzv3.ExtAuthz{}
		Expect(filterByName(filters, filterExtAuthz).GetTypedConfig().UnmarshalTo(cfg)).To(Succeed())

		Expect(cfg.GetGrpcService().GetEnvoyGrpc().GetClusterName()).To(Equal(lmsIssuerCluster))
		// fail closed: request rejected when issuer is unreachable.
		Expect(cfg.GetFailureModeAllow()).To(BeFalse())
		// issuer receives the jwt_authn verified payload.
		Expect(cfg.GetMetadataContextNamespaces()).To(ContainElement(filterJwtAuthn))
	})
})

var _ = Describe("lmsVhostPerFilterConfig", func() {
	It("returns nil when LMS is disabled", func() {
		m, err := lmsVhostPerFilterConfig(lmsIntent{enabled: false}, customScopesIntent{})
		Expect(err).NotTo(HaveOccurred())
		Expect(m).To(BeNil())
	})

	It("carries realm and environment as ext_authz context_extensions", func() {
		m, err := lmsVhostPerFilterConfig(lmsIntent{enabled: true, realm: "realm1", environment: "prod"}, customScopesIntent{})
		Expect(err).NotTo(HaveOccurred())
		Expect(m).To(HaveKey(filterExtAuthz))

		perRoute := &extauthzv3.ExtAuthzPerRoute{}
		Expect(m[filterExtAuthz].UnmarshalTo(perRoute)).To(Succeed())
		ext := perRoute.GetCheckSettings().GetContextExtensions()
		Expect(ext).To(HaveKeyWithValue("realm", "realm1"))
		Expect(ext).To(HaveKeyWithValue("environment", "prod"))
	})
})

var _ = Describe("buildLMSIssuerCluster", func() {
	It("is an http2 STRICT_DNS cluster to the issuer host:port", func() {
		c, err := buildLMSIssuerCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(c.GetName()).To(Equal(lmsIssuerCluster))

		ep := c.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].
			GetEndpoint().GetAddress().GetSocketAddress()
		Expect(ep.GetAddress()).To(Equal(lmsIssuerHost))
		Expect(ep.GetPortValue()).To(Equal(uint32(lmsIssuerPort)))

		// gRPC requires HTTP/2 upstream protocol options.
		raw := c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
		Expect(raw).NotTo(BeNil())
		opts := &upstreamsv3.HttpProtocolOptions{}
		Expect(raw.UnmarshalTo(opts)).To(Succeed())
		Expect(opts.GetExplicitHttpConfig().GetHttp2ProtocolOptions()).NotTo(BeNil())
	})
})

var _ = Describe("LastMileSecurityFeature", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = contextutil.WithEnv(context.Background(), "poc") })

	newRoute := func() *gatewayv1.Route {
		r := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{
				Security: gatewayv1.Security{RealmName: "poc-realm"},
			},
		}
		return r
	}

	It("is used for a normal route", func() {
		b := NewFeatureBuilder(nil, newRoute(), nil, nil)
		Expect(InstanceLastMileSecurityFeature.IsUsed(ctx, b)).To(BeTrue())
	})

	It("is not used for a pass-through route", func() {
		r := newRoute()
		r.Spec.PassThrough = true
		b := NewFeatureBuilder(nil, r, nil, nil)
		Expect(InstanceLastMileSecurityFeature.IsUsed(ctx, b)).To(BeFalse())
	})

	It("is not used for a failover route", func() {
		r := newRoute()
		r.Spec.Traffic.Failover = &gatewayv1.Failover{}
		b := NewFeatureBuilder(nil, r, nil, nil)
		Expect(InstanceLastMileSecurityFeature.IsUsed(ctx, b)).To(BeFalse())
	})

	It("declares LMS intent with realm and environment", func() {
		b := NewFeatureBuilder(nil, newRoute(), nil, nil).(*Builder)
		Expect(InstanceLastMileSecurityFeature.Apply(ctx, b)).To(Succeed())
		Expect(b.lms.enabled).To(BeTrue())
		Expect(b.lms.realm).To(Equal("poc-realm"))
		Expect(b.lms.environment).To(Equal("poc"))
	})

	It("publishes a consistent snapshot including the issuer cluster", func() {
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		xds := NewXdsClient(cache)
		b := NewFeatureBuilder(xds, newRoute(), nil, nil)
		b.EnableFeature(InstanceLastMileSecurityFeature)
		b.SetUpstream(client.NewUpstreamOrDie("https://backend.svc.local:8080/api"))

		Expect(b.Build(ctx)).To(Succeed())

		snap, err := cache.GetSnapshot(PocNodeID)
		Expect(err).NotTo(HaveOccurred())
		// upstream cluster + issuer cluster (no trusted issuers => no jwks).
		Expect(snap.GetResources(resource.ClusterType)).To(HaveKey(lmsIssuerCluster))
	})
})
