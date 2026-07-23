// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"

	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	upstreamsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ = Describe("Envoy rate limiting", func() {
	It("builds route and consumer descriptors for every configured window", func() {
		intent := newRateLimitIntent("ns/checkout", &gatewayv1.RateLimit{
			Limits: gatewayv1.Limits{Second: 10, Minute: 100},
		}, map[string]gatewayv1.Limits{
			"consumer-b": {Hour: 200},
			"consumer-a": {Minute: 20},
		})

		descriptors := buildRateLimitDescriptors(intent)
		Expect(descriptors).To(HaveLen(4))
		Expect(descriptors[0].GetActions()[0].GetGenericKey().GetDescriptorValue()).To(Equal("ns/checkout"))
		Expect(descriptors[0].GetActions()[1].GetGenericKey().GetDescriptorValue()).To(Equal("second"))
		Expect(descriptors[2].GetActions()[1].GetHeaderValueMatch().GetDescriptorValue()).To(Equal("consumer-a"))
		Expect(descriptors[2].GetActions()[2].GetGenericKey().GetDescriptorValue()).To(Equal("minute"))
		Expect(descriptors[3].GetActions()[1].GetHeaderValueMatch().GetDescriptorValue()).To(Equal("consumer-b"))
	})

	It("configures fail-open and hides rate-limit headers from route options", func() {
		intent := newRateLimitIntent("ns/checkout", &gatewayv1.RateLimit{
			Limits: gatewayv1.Limits{Minute: 100},
			Options: gatewayv1.RateLimitOptions{
				FaultTolerant:     true,
				HideClientHeaders: true,
			},
		}, nil)

		filter := buildRateLimitFilter(intent)
		Expect(filter.GetFailureModeDeny()).To(BeFalse())
		Expect(filter.GetDisableXEnvoyRatelimitedHeader()).To(BeTrue())
		Expect(filter.GetRateLimitService().GetGrpcService().GetEnvoyGrpc().GetClusterName()).To(Equal(rateLimitCluster))
	})

	It("orders rate limiting after authorization and before LMS", func() {
		filters, err := buildFilters(
			accessControlIntent{trustedIssuers: []string{"https://issuer"}, accessControl: true},
			rateLimitIntent{enabled: true, faultTolerant: true},
			lmsIntent{enabled: true},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveLen(6))
		Expect(filters[0].GetName()).To(Equal(filterJwtAuthn))
		Expect(filters[1].GetName()).To(Equal(filterRBAC))
		Expect(filters[2].GetName()).To(Equal(filterSetMetadata))
		Expect(filters[3].GetName()).To(Equal(filterRateLimit))
		Expect(filters[4].GetName()).To(Equal(filterExtAuthz))
		Expect(filters[5].GetName()).To(Equal(filterRouter))

		config := &ratelimitfilterv3.RateLimit{}
		Expect(filters[3].GetTypedConfig().UnmarshalTo(config)).To(Succeed())
		Expect(config.GetDomain()).To(Equal(rateLimitDomain))
	})

	It("builds an HTTP/2 cluster for the gRPC rate-limit service", func() {
		cluster, err := buildRateLimitCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(cluster.GetName()).To(Equal(rateLimitCluster))
		address := cluster.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].
			GetEndpoint().GetAddress().GetSocketAddress()
		Expect(address.GetAddress()).To(Equal(rateLimitHost))
		Expect(address.GetPortValue()).To(Equal(uint32(rateLimitPort)))

		options := &upstreamsv3.HttpProtocolOptions{}
		raw := cluster.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
		Expect(raw.UnmarshalTo(options)).To(Succeed())
		Expect(options.GetExplicitHttpConfig().GetHttp2ProtocolOptions()).NotTo(BeNil())
	})

	It("collects only matching consumer limits", func() {
		route := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{Security: gatewayv1.Security{
				TrustedIssuers: []string{"https://issuer"},
			}},
		}
		matching := &gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
			Route:        ctypes.ObjectRef{Name: "checkout", Namespace: "ns"},
			ConsumerName: "consumer-a",
			Traffic: &gatewayv1.ConsumeRouteTraffic{RateLimit: &gatewayv1.ConsumeRouteRateLimit{
				Limits: gatewayv1.Limits{Minute: 20},
			}},
		}}
		other := matching.DeepCopy()
		other.Spec.Route.Name = "other"
		other.Spec.ConsumerName = "consumer-b"

		builder := NewFeatureBuilder(nil, route, nil, nil).(*Builder)
		builder.AddAllowedConsumers(matching, other)
		Expect(InstanceRateLimitFeature.Apply(context.Background(), builder)).To(Succeed())
		Expect(builder.rateLimit.routeID).To(Equal("ns/checkout"))
		Expect(builder.rateLimit.consumerLimits).To(HaveKey("consumer-a"))
		Expect(builder.rateLimit.consumerLimits).NotTo(HaveKey("consumer-b"))
	})
})
