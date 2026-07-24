// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy_test

import (
	"context"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/gateway/internal/features/envoy"
)

var _ = Describe("XdsCache", func() {
	const nodeID = "test-gateway"

	var (
		ctx   context.Context
		xds   *envoy.XdsCache
		cache cachev3.SnapshotCache
	)

	BeforeEach(func() {
		ctx = context.Background()
		xds = envoy.NewXdsCache(GinkgoLogr)
		var ok bool
		cache, ok = xds.Cache().(cachev3.SnapshotCache)
		Expect(ok).To(BeTrue())
	})

	versionOf := func() string {
		snap, err := cache.GetSnapshot(nodeID)
		Expect(err).NotTo(HaveOccurred())
		return snap.GetVersion(resourcev3.ClusterType)
	}

	bundleWith := func(clusterName string) envoy.ResourceBundle {
		return envoy.ResourceBundle{
			Clusters: []*clusterv3.Cluster{{Name: clusterName}},
		}
	}

	It("publishes a snapshot for a node", func() {
		Expect(xds.SetSnapshotFor(ctx, nodeID, bundleWith("a"))).To(Succeed())

		snap, err := cache.GetSnapshot(nodeID)
		Expect(err).NotTo(HaveOccurred())
		Expect(snap.GetResources(resourcev3.ClusterType)).To(HaveKey("a"))
	})

	It("does not re-publish when the bundle is unchanged (hash-diff gate)", func() {
		Expect(xds.SetSnapshotFor(ctx, nodeID, bundleWith("a"))).To(Succeed())
		v1 := versionOf()

		Expect(xds.SetSnapshotFor(ctx, nodeID, bundleWith("a"))).To(Succeed())
		v2 := versionOf()

		Expect(v2).To(Equal(v1), "identical content must yield the same version")
	})

	It("re-publishes with a new version when the bundle changes", func() {
		Expect(xds.SetSnapshotFor(ctx, nodeID, bundleWith("a"))).To(Succeed())
		v1 := versionOf()

		Expect(xds.SetSnapshotFor(ctx, nodeID, bundleWith("b"))).To(Succeed())
		v2 := versionOf()

		Expect(v2).NotTo(Equal(v1), "changed content must yield a different version")
	})
})
