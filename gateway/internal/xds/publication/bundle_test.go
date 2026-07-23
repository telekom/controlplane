// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package publication

import (
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/gateway/internal/features/envoy"
	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BundleFromResources", func() {
	It("builds a deterministic canonical publication envelope", func() {
		resources := &envoy.ResourceBundle{
			Target: envoy.TargetIdentity{
				Environment: "test", Namespace: "team", Name: "gateway", UID: types.UID("uid-1"),
			},
			Source: envoy.SourceMetadata{Resources: []envoy.SourceReference{
				{Kind: "Route", Namespace: "team", Name: "b", UID: types.UID("b"), Generation: 2},
				{Kind: "Gateway", Namespace: "team", Name: "gateway", UID: types.UID("uid-1"), Generation: 1},
			}},
			Listeners: []*listenerv3.Listener{{Name: "listener"}},
		}

		first, err := BundleFromResources(resources)
		Expect(err).NotTo(HaveOccurred())
		second, err := BundleFromResources(resources)
		Expect(err).NotTo(HaveOccurred())

		Expect(first.TargetId).To(Equal("test/team/gateway/uid-1"))
		Expect(first.SchemaVersion).To(Equal(xdsapi.SchemaVersion))
		Expect(first.PublisherGeneration).To(Equal(second.PublisherGeneration))
		Expect(first.Digest).To(Equal(second.Digest))
		Expect(first.Sources[0].Kind).To(Equal("Gateway"))
		Expect(first.Listeners).To(HaveLen(1))
	})

	It("rejects an incomplete target", func() {
		_, err := BundleFromResources(&envoy.ResourceBundle{})
		Expect(err).To(MatchError(ContainSubstring("target")))
	})

	It("canonicalizes nested map-bearing protobuf values", func() {
		firstStruct, err := structpb.NewStruct(map[string]any{"a": "one", "b": "two"})
		Expect(err).NotTo(HaveOccurred())
		secondStruct, err := structpb.NewStruct(map[string]any{"b": "two", "a": "one"})
		Expect(err).NotTo(HaveOccurred())
		first := &envoy.ResourceBundle{
			Target:    envoy.TargetIdentity{Environment: "test", Namespace: "team", Name: "gateway", UID: "uid"},
			Source:    envoy.SourceMetadata{Resources: []envoy.SourceReference{{Kind: "Gateway", Name: "gateway"}}},
			Listeners: []*listenerv3.Listener{{Name: "listener", Metadata: &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"test": firstStruct}}}},
		}
		second := &envoy.ResourceBundle{
			Target: first.Target, Source: first.Source,
			Listeners: []*listenerv3.Listener{{Name: "listener", Metadata: &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"test": secondStruct}}}},
		}
		firstBundle, err := BundleFromResources(first)
		Expect(err).NotTo(HaveOccurred())
		secondBundle, err := BundleFromResources(second)
		Expect(err).NotTo(HaveOccurred())
		Expect(firstBundle.Digest).To(Equal(secondBundle.Digest))
	})
})
