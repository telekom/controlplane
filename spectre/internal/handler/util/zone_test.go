// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"errors"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mock "github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// isBlockedError checks if the error implements the BlockedError interface.
func isBlockedError(err error) bool {
	var be ctrlerrors.BlockedError
	ok := errors.As(err, &be)
	return ok && be.IsBlocked()
}

// makeZone creates a Zone with the given name.
func makeZone(name string) *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-env",
		},
	}
}

// makeReadyEventConfig creates a ready EventConfig for the given zone with optional mesh zone names.
func makeReadyEventConfig(name, zoneName string, meshZones ...string) eventv1.EventConfig {
	ec := eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-env--" + zoneName,
		},
		Spec: eventv1.EventConfigSpec{
			Zone: ctypes.ObjectRef{Name: zoneName, Namespace: "test-env"},
			Local: &eventv1.LocalBackend{
				Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
				ServerSendEventUrl: "http://sse.local",
				PublishEventUrl:    "http://publish.local",
			},
		},
		Status: eventv1.EventConfigStatus{
			CallbackURL: "https://gateway.example.com/horizon/callback/v1",
		},
	}

	if len(meshZones) > 0 {
		ec.Spec.Mesh = &eventv1.MeshConfig{
			FullMesh:  false,
			ZoneNames: meshZones,
		}
	}

	// Set Ready condition
	ec.Status.Conditions = []metav1.Condition{
		{
			Type:   condition.ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Provisioned",
		},
	}
	return ec
}

// sequentialListResponder creates a mock Run function that returns different EventConfig lists
// for sequential calls. Each call consumes the next response in the sequence.
func sequentialListResponder(responses ...[]eventv1.EventConfig) func(context.Context, client.ObjectList, ...client.ListOption) {
	var callCount atomic.Int32
	return func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
		idx := int(callCount.Add(1)) - 1
		if idx < len(responses) {
			*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: responses[idx]}
		} else {
			*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{}}
		}
	}
}

var _ = Describe("GetListeningZone", func() {
	var (
		ctx          context.Context
		fakeClient   *fakeclient.MockJanitorClient
		providerZone *adminv1.Zone
		consumerZone *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		providerZone = makeZone("provider-zone")
		consumerZone = makeZone("consumer-zone")
	})

	It("should use the consumer zone when its EventConfig supports the provider zone (full mesh)", func() {
		// Consumer (= listener) zone has an EventConfig with full mesh, so it supports all zones.
		ec := makeReadyEventConfig("ec-consumer", "consumer-zone")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(sequentialListResponder([]eventv1.EventConfig{ec})).
			Return(nil).
			Once()

		result, err := util.GetListeningZone(ctx, providerZone, consumerZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Name).To(Equal("consumer-zone"))
	})

	It("should use the consumer zone when its restricted mesh names the provider zone", func() {
		ec := makeReadyEventConfig("ec-consumer", "consumer-zone", "provider-zone")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(sequentialListResponder([]eventv1.EventConfig{ec})).
			Return(nil).
			Once()

		result, err := util.GetListeningZone(ctx, providerZone, consumerZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Name).To(Equal("consumer-zone"))
	})

	It("should fall back to the provider zone when the consumer mesh excludes the provider zone", func() {
		// Restricted mesh that does NOT name the provider zone: the consumer zone cannot
		// carry this subscription, so the provider zone must win.
		ecConsumer := makeReadyEventConfig("ec-consumer", "consumer-zone", "other-zone")
		ecProvider := makeReadyEventConfig("ec-provider", "provider-zone")
		responder := sequentialListResponder(
			[]eventv1.EventConfig{ecConsumer}, // consumer zone: mesh excludes provider zone
			[]eventv1.EventConfig{ecProvider}, // provider zone: has EventConfig
		)

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(responder).
			Return(nil).
			Times(2)

		result, err := util.GetListeningZone(ctx, providerZone, consumerZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Name).To(Equal("provider-zone"))
	})

	It("should fall back to the provider zone when the consumer zone has no EventConfig", func() {
		ecProvider := makeReadyEventConfig("ec-provider", "provider-zone")
		responder := sequentialListResponder(
			[]eventv1.EventConfig{},           // consumer zone: no EventConfig
			[]eventv1.EventConfig{ecProvider}, // provider zone: has EventConfig
		)

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(responder).
			Return(nil).
			Times(2)

		result, err := util.GetListeningZone(ctx, providerZone, consumerZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Name).To(Equal("provider-zone"))
	})

	It("should return BlockedError when no zone has an EventConfig", func() {
		responder := sequentialListResponder(
			[]eventv1.EventConfig{}, // consumer zone: no EventConfig
			[]eventv1.EventConfig{}, // provider zone: no EventConfig
		)

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(responder).
			Return(nil).
			Times(2)

		result, err := util.GetListeningZone(ctx, providerZone, consumerZone)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("no zone has an EventStore"))
	})
})

var _ = Describe("GetEventConfig", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		zone       *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		zone = makeZone("zone-a")
	})

	It("should return EventConfig when found and ready", func() {
		ec := makeReadyEventConfig("ec-zone-a", "zone-a")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil).
			Once()

		result, err := util.GetEventConfig(ctx, zone)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("ec-zone-a"))
	})

	It("should return BlockedError when no EventConfig found", func() {
		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{}}
			}).
			Return(nil).
			Once()

		result, err := util.GetEventConfig(ctx, zone)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("no EventConfig found"))
	})

	It("should return BlockedError when EventConfig is not ready", func() {
		ec := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ec-zone-a",
				Namespace: "test-env--zone-a",
			},
			Spec: eventv1.EventConfigSpec{
				Zone: ctypes.ObjectRef{Name: "zone-a", Namespace: "test-env"},
				Local: &eventv1.LocalBackend{
					Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
					ServerSendEventUrl: "http://sse.local",
					PublishEventUrl:    "http://publish.local",
				},
			},
		}
		// No Ready condition set = not ready

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil).
			Once()

		result, err := util.GetEventConfig(ctx, zone)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})
