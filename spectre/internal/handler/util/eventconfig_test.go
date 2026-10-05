// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"errors"
	"fmt"

	mock "github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

// makeReadyEventConfig creates a ready EventConfig for the given zone.
func makeReadyEventConfig(name, zoneName string) eventv1.EventConfig {
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

// makeReadyEventStore creates an EventStore with a Ready condition.
func makeReadyEventStore(name, namespace string) *pubsubv1.EventStore {
	return &pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: pubsubv1.EventStoreSpec{
			Url:          "http://admin.local",
			TokenUrl:     "http://token.local",
			ClientId:     "client-id",
			ClientSecret: "client-secret",
		},
		Status: pubsubv1.EventStoreStatus{
			Conditions: []metav1.Condition{
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Provisioned",
				},
			},
		},
	}
}

// makeReadyEventConfigWithEventStore creates a ready EventConfig that has a Status.EventStore reference.
func makeReadyEventConfigWithEventStore(name, zoneName string, eventStoreRef *ctypes.ObjectRef) eventv1.EventConfig {
	ec := makeReadyEventConfig(name, zoneName)
	ec.Status.EventStore = eventStoreRef
	return ec
}

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

var _ = Describe("GetEventConfig (duplicate rejection)", func() {
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

	It("should return error when multiple EventConfigs exist for the zone", func() {
		ec1 := makeReadyEventConfig("ec-1", "zone-a")
		ec2 := makeReadyEventConfig("ec-2", "zone-a")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec1, ec2}}
			}).
			Return(nil).
			Once()

		result, err := util.GetEventConfig(ctx, zone)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("found 2 EventConfigs"))
		// Ambiguity is a hard error, not a blocked error
		Expect(err).ToNot(Satisfy(isBlockedError))
	})
})

var _ = Describe("FindEventConfig", func() {
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

	mockList := func(items ...eventv1.EventConfig) {
		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: items}
			}).
			Return(nil).
			Once()
	}

	It("should return nil without error when the zone has no EventConfig", func() {
		mockList()
		result, err := util.FindEventConfig(ctx, zone)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("should return the EventConfig without checking readiness", func() {
		ec := makeReadyEventConfig("ec-1", "zone-a")
		ec.Status.Conditions = nil
		mockList(ec)
		result, err := util.FindEventConfig(ctx, zone)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("ec-1"))
	})

	It("should return a non-blocked error for two EventConfigs", func() {
		mockList(makeReadyEventConfig("ec-1", "zone-a"), makeReadyEventConfig("ec-2", "zone-a"))
		result, err := util.FindEventConfig(ctx, zone)
		Expect(err).To(MatchError(ContainSubstring("found 2 EventConfigs")))
		Expect(err).ToNot(Satisfy(isBlockedError))
		Expect(result).To(BeNil())
	})

	It("should wrap List errors", func() {
		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Return(fmt.Errorf("connection refused")).
			Once()
		result, err := util.FindEventConfig(ctx, zone)
		Expect(err).To(MatchError(ContainSubstring("connection refused")))
		Expect(err).ToNot(Satisfy(isBlockedError))
		Expect(result).To(BeNil())
	})
})

var _ = Describe("ResolveEventStore", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return EventStore when EventConfig references a ready EventStore", func() {
		esRef := &ctypes.ObjectRef{Name: "es-zone-a", Namespace: "test-env--zone-a"}
		ec := makeReadyEventConfigWithEventStore("ec-zone-a", "zone-a", esRef)
		es := makeReadyEventStore("es-zone-a", "test-env--zone-a")

		fakeClient.EXPECT().
			Get(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				*obj.(*pubsubv1.EventStore) = *es
			}).
			Return(nil).
			Once()

		result, err := util.ResolveEventStore(ctx, &ec)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("es-zone-a"))
	})

	It("should return BlockedError when EventConfig has no Status.EventStore", func() {
		ec := makeReadyEventConfig("ec-zone-a", "zone-a")
		// Status.EventStore is nil by default from makeReadyEventConfig

		result, err := util.ResolveEventStore(ctx, &ec)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no EventStore reference"))
	})

	It("should return error when referenced EventStore is not found", func() {
		esRef := &ctypes.ObjectRef{Name: "es-zone-a", Namespace: "test-env--zone-a"}
		ec := makeReadyEventConfigWithEventStore("ec-zone-a", "zone-a", esRef)

		fakeClient.EXPECT().
			Get(mock.Anything, mock.Anything, mock.Anything).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.2.2.2.2", Resource: "eventstores"}, "es-zone-a")).
			Once()

		result, err := util.ResolveEventStore(ctx, &ec)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get EventStore"))
	})

	It("should return BlockedError when referenced EventStore is not ready", func() {
		esRef := &ctypes.ObjectRef{Name: "es-zone-a", Namespace: "test-env--zone-a"}
		ec := makeReadyEventConfigWithEventStore("ec-zone-a", "zone-a", esRef)

		// EventStore exists but has no Ready condition
		esNotReady := &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "es-zone-a",
				Namespace: "test-env--zone-a",
			},
			Spec: pubsubv1.EventStoreSpec{
				Url:          "http://admin.local",
				TokenUrl:     "http://token.local",
				ClientId:     "client-id",
				ClientSecret: "client-secret",
			},
		}

		fakeClient.EXPECT().
			Get(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				*obj.(*pubsubv1.EventStore) = *esNotReady
			}).
			Return(nil).
			Once()

		result, err := util.ResolveEventStore(ctx, &ec)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})
