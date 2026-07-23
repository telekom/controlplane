// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"time"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("xDS demo helpers", func() {
	It("waits for and trims the target handoff", func() {
		path := filepath.Join(GinkgoT().TempDir(), "state", "target-id")
		go func() {
			time.Sleep(20 * time.Millisecond)
			Expect(os.MkdirAll(filepath.Dir(path), 0o750)).To(Succeed())
			Expect(os.WriteFile(path, []byte("demo/ns/gateway/uid\n"), 0o600)).To(Succeed())
		}()

		target, err := waitForTarget(path, time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(target).To(Equal("demo/ns/gateway/uid"))
	})

	It("forwards management-server flags after adding both node mappings", func() {
		args := managementServerArgs("/management", "demo/ns/gateway/uid", []string{
			"-database=/data/xds.db", "-grpc-address=:18000",
		})

		Expect(args).To(Equal([]string{
			"/management",
			"-node-mappings=demo-envoy-a=demo/ns/gateway/uid,demo-envoy-b=demo/ns/gateway/uid",
			"-database=/data/xds.db",
			"-grpc-address=:18000",
		}))
	})

	It("builds pass-through and secured real Route specs", func() {
		plain := demoRouteSpec(routeRequest{Path: "/v1", Host: "demo.local"})
		Expect(plain.PassThrough).To(BeTrue())
		Expect(plain.Backend.Upstreams).To(HaveLen(1))
		Expect(plain.Backend.Upstreams[0].Hostname).To(Equal("172.30.0.10"))
		Expect(plain.Backend.Upstreams[0].Path).To(Equal("/anything"))
		Expect(plain.Security.TrustedIssuers).To(BeEmpty())

		secured := demoRouteSpec(routeRequest{
			Path: "/secure", Host: "demo.local", Issuer: "https://issuer.example", Consumer: "client-a",
		})
		Expect(secured.PassThrough).To(BeFalse())
		Expect(secured.Security.TrustedIssuers).To(Equal([]string{"https://issuer.example"}))
		Expect(secured.Security.DefaultConsumers).To(Equal([]string{"client-a"}))
	})

	It("exposes independent node observations", func() {
		view := statusView(&xdsapi.GetStatusResponse{
			TargetId: "target", PersistedGeneration: 3, ConnectedNodeIds: []string{"node-a", "node-b"},
			Observations: []*xdsapi.DeliveryObservation{
				{NodeId: "node-a", Generation: 3, State: xdsapi.DeliveryState_DELIVERY_STATE_ACK},
				{NodeId: "node-b", Generation: 3, State: xdsapi.DeliveryState_DELIVERY_STATE_ACK},
			},
		})
		Expect(view.ConnectedNode).To(ConsistOf("node-a", "node-b"))
		Expect(view.Observations).To(HaveLen(2))
		Expect(view.Observations[0]["nodeId"]).To(Equal("node-a"))
		Expect(view.Observations[1]["nodeId"]).To(Equal("node-b"))
	})
})
