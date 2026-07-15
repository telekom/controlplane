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

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// readyPeerZone builds a Ready adminv1.Zone (GetZone requires readiness).
func readyPeerZone(name string) *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status:     adminv1.ZoneStatus{Namespace: "default"},
	}
	meta.SetStatusCondition(&z.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return z
}

func peerEventConfig(name, zoneName string, mesh *eventv1.MeshConfig, proxy *eventv1.ProxyBackend) eventv1.EventConfig {
	return eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: eventv1.EventConfigSpec{
			Zone:  ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			Mesh:  mesh,
			Proxy: proxy,
		},
	}
}

func peerZoneNames(zones []*adminv1.Zone) []string {
	names := make([]string, len(zones))
	for i, z := range zones {
		names[i] = z.Name
	}
	return names
}

var _ = Describe("listMeshPeerZones inbound filter", func() {
	var (
		ctx context.Context
		fc  *fakeclient.MockJanitorClient
		obj *eventv1.EventConfig
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test-env")
		fc = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fc)

		obj = &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "self", Namespace: "default"},
			Spec:       eventv1.EventConfigSpec{Zone: ctypes.ObjectRef{Name: "test-zone", Namespace: "default"}},
		}

		// real+inbound: non-proxy, FullMesh -> SupportsZone(test-zone)=true
		peerReal := peerEventConfig("peer-real", "zone-real", &eventv1.MeshConfig{FullMesh: true}, nil)
		// proxy+inbound: proxy peer whose partial mesh lists test-zone -> inbound but not real
		peerProxy := peerEventConfig("peer-proxy", "zone-proxy",
			&eventv1.MeshConfig{FullMesh: false, ZoneNames: []string{"test-zone"}},
			&eventv1.ProxyBackend{TargetZone: ctypes.ObjectRef{Name: "zone-real", Namespace: "default"}})
		// real but NOT inbound: non-proxy partial mesh that does not list test-zone
		peerOut := peerEventConfig("peer-out", "zone-out",
			&eventv1.MeshConfig{FullMesh: false, ZoneNames: []string{"somewhere-else"}}, nil)

		fc.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{
					Items: []eventv1.EventConfig{peerReal, peerProxy, peerOut},
				}
			}).
			Return(nil).Once()

		for _, zn := range []string{"zone-real", "zone-proxy", "zone-out"} {
			zone := readyPeerZone(zn)
			fc.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: zn, Namespace: "default"}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *zone
				}).
				Return(nil).Once()
		}
	})

	It("includes only peers that mesh with obj's zone in inboundPeerZones", func() {
		h := &EventConfigHandler{}
		realPeers, allPeers, inboundPeers, err := h.listMeshPeerZones(ctx, obj)
		Expect(err).ToNot(HaveOccurred())

		// realPeerZones: non-proxy peers only (proxy peer excluded).
		Expect(peerZoneNames(realPeers)).To(ConsistOf("zone-real", "zone-out"))
		// allPeerZones: every peer.
		Expect(peerZoneNames(allPeers)).To(ConsistOf("zone-real", "zone-proxy", "zone-out"))
		// inboundPeerZones: only peers whose SupportsZone(test-zone) is true.
		// zone-out does not mesh with test-zone and must be excluded.
		Expect(peerZoneNames(inboundPeers)).To(ConsistOf("zone-real", "zone-proxy"))
	})
})
