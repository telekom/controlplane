// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/discovery-server/internal/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFailoverFiltering(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Failover filtering")
}

func failoverZone(name, ns string) *adminv1.Zone {
	z := &adminv1.Zone{}
	z.Name = name
	z.Status.Namespace = ns
	z.Spec.Gateways = []adminv1.GatewayConfig{{Name: "standard"}}
	z.Spec.Presets = []adminv1.Preset{{
		Name: "failover", Type: adminv1.GatewayTypeAPI, GatewayRef: "standard",
		Urls:     []adminv1.UrlConfig{{Hostname: name + ".example", Scheme: "https"}},
		Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}},
	}}
	return z
}

func failoverSub(namespaces ...string) *apiv1.ApiSubscription {
	s := &apiv1.ApiSubscription{}
	s.Spec.ApiBasePath = "/api/v1"
	s.Spec.Traffic.Failover = &apiv1.SubscriberFailover{Enabled: true}
	for _, ns := range namespaces {
		s.Status.FailoverRoutes = append(s.Status.FailoverRoutes, ctypes.ObjectRef{Name: "route", Namespace: ns})
	}
	return s
}

func namespaceQuery(ns, cursor string) any {
	return mock.MatchedBy(func(opts store.ListOpts) bool {
		return len(opts.Filters) == 1 && opts.Filters[0] == (store.Filter{Path: "status.namespace", Op: store.OpEqual, Value: ns}) && opts.Cursor == cursor
	})
}

var _ = Describe("Failover zone lookup", func() {
	It("queries only distinct route namespaces and preserves route order", func() {
		s := csmocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
		s.EXPECT().List(mock.Anything, namespaceQuery("zone-b", "")).Return(&store.ListResponse[*adminv1.Zone]{Items: []*adminv1.Zone{failoverZone("b", "zone-b")}}, nil).Once()
		s.EXPECT().List(mock.Anything, namespaceQuery("zone-a", "")).Return(&store.ListResponse[*adminv1.Zone]{Items: []*adminv1.Zone{failoverZone("a", "zone-a")}}, nil).Once()
		s.EXPECT().List(mock.Anything, namespaceQuery("unknown", "")).Return(&store.ListResponse[*adminv1.Zone]{}, nil).Once()
		out := api.ApiSubscriptionResponse{}
		mapFailover(context.Background(), failoverSub("zone-b", "zone-a", "unknown", "zone-b"), &out, s)
		Expect(out.Failover).To(HaveLen(3))
		Expect(out.Failover[0].GatewayUrl).To(Equal("https://b.example/api/v1"))
		Expect(out.Failover[1].GatewayUrl).To(Equal("https://a.example/api/v1"))
		Expect(out.Failover[2].GatewayUrl).To(Equal("https://b.example/api/v1"))
	})

	It("keeps the namespace filter on subsequent pages", func() {
		s := csmocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
		first := make([]*adminv1.Zone, 500)
		for i := range first {
			first[i] = failoverZone("old", "zone-a")
		}
		s.EXPECT().List(mock.Anything, namespaceQuery("zone-a", "")).Return(&store.ListResponse[*adminv1.Zone]{Items: first, Links: store.ListResponseLinks{Next: "next-key"}}, nil).Once()
		s.EXPECT().List(mock.Anything, namespaceQuery("zone-a", "next-key")).Return(&store.ListResponse[*adminv1.Zone]{Items: []*adminv1.Zone{failoverZone("latest", "zone-a")}}, nil).Once()
		out := api.ApiSubscriptionResponse{}
		mapFailover(context.Background(), failoverSub("zone-a"), &out, s)
		Expect(out.Failover).To(HaveLen(1))
		Expect(out.Failover[0].Zone).To(Equal("latest"))
	})

	It("leaves failover unset on lookup errors", func() {
		s := csmocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
		s.EXPECT().List(mock.Anything, namespaceQuery("zone-a", "")).Return(nil, context.Canceled).Once()
		out := api.ApiSubscriptionResponse{}
		mapFailover(context.Background(), failoverSub("zone-a"), &out, s)
		Expect(out.Failover).To(BeNil())
	})

	It("does not list zones without failover routes", func() {
		s := csmocks.NewMockObjectStore[*adminv1.Zone](GinkgoT())
		out := api.ApiSubscriptionResponse{}
		mapFailover(context.Background(), failoverSub(), &out, s)
		Expect(out.Failover).To(BeNil())
	})
})
