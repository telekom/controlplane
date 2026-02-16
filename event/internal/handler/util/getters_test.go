// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mock "github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// unwrapAll follows the pkg/errors Cause chain to the root error.
func unwrapAll(err error) error {
	for {
		cause, ok := err.(interface{ Cause() error })
		if !ok {
			return err
		}
		err = cause.Cause()
	}
}

// isBlockedError checks if the error implements the BlockedError interface.
func isBlockedError(err error) bool {
	be, ok := err.(ctrlerrors.BlockedError)
	return ok && be.IsBlocked()
}

// setReady sets the Ready condition to True on a resource.
func setReady(obj ctypes.Object) {
	meta.SetStatusCondition(conditionsPtr(obj), metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
}

// conditionsPtr returns a pointer to the conditions slice for the given object.
func conditionsPtr(obj ctypes.Object) *[]metav1.Condition {
	switch o := obj.(type) {
	case *adminv1.Zone:
		return &o.Status.Conditions
	case *eventv1.EventConfig:
		return &o.Status.Conditions
	case *eventv1.EventType:
		return &o.Status.Conditions
	case *eventv1.EventExposure:
		return &o.Status.Conditions
	case *applicationapi.Application:
		return &o.Status.Conditions
	case *pubsubv1.EventStore:
		return &o.Status.Conditions
	default:
		panic(fmt.Sprintf("unsupported type %T", obj))
	}
}

// ---------- GetZone ----------

var _ = Describe("GetZone", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		ref        client.ObjectKey
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		ref = client.ObjectKey{Name: "zone-a", Namespace: "default"}
	})

	It("should return zone when found and ready", func() {
		expected := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "zone-a", Namespace: "default"},
		}
		setReady(expected)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *expected
			}).
			Return(nil)

		result, err := util.GetZone(ctx, ref)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("zone-a"))
	})

	It("should return BlockedError when zone is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "admin.cp.ei.telekom.de", Resource: "zones"}, "zone-a"))

		result, err := util.GetZone(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return wrapped error on unexpected Get failure", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Return(fmt.Errorf("connection refused"))

		result, err := util.GetZone(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get zone"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return BlockedError when zone is not ready", func() {
		notReady := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "zone-a", Namespace: "default"},
		}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "zone-a", Namespace: "default"}, &adminv1.Zone{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *notReady
			}).
			Return(nil)

		result, err := util.GetZone(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- GetEventConfigForZone ----------

var _ = Describe("GetEventConfigForZone", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return EventConfig when single match found and ready", func() {
		ec := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			Spec:       eventv1.EventConfigSpec{Zone: ctypes.ObjectRef{Name: "zone-a", Namespace: "default"}},
		}
		setReady(&ec)

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil)

		result, err := util.GetEventConfigForZone(ctx, "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("ec-zone-a"))
	})

	It("should return wrapped error on List failure", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Return(fmt.Errorf("api server unavailable"))

		result, err := util.GetEventConfigForZone(ctx, "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list EventConfigs"))
		Expect(err.Error()).To(ContainSubstring("api server unavailable"))
	})

	It("should return BlockedError when no EventConfig found", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{}}
			}).
			Return(nil)

		result, err := util.GetEventConfigForZone(ctx, "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("no EventConfig found"))
	})

	It("should pick the first (oldest) when multiple EventConfigs found", func() {
		now := metav1.Now()
		later := metav1.NewTime(now.Add(time.Minute))

		ec1 := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "ec-old",
				Namespace:         "default",
				CreationTimestamp: now,
			},
		}
		setReady(&ec1)

		ec2 := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "ec-new",
				Namespace:         "default",
				CreationTimestamp: later,
			},
		}
		setReady(&ec2)

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec2, ec1}}
			}).
			Return(nil)

		result, err := util.GetEventConfigForZone(ctx, "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("ec-old"))
	})

	It("should return BlockedError when EventConfig is not ready", func() {
		ec := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
		}
		// Not ready — no condition set

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil)

		result, err := util.GetEventConfigForZone(ctx, "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- GetEventStoreForZone ----------

var _ = Describe("GetEventStoreForZone", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return EventStore when EventConfig and EventStore both found and ready", func() {
		esRef := ctypes.ObjectRef{Name: "es-1", Namespace: "default"}
		ec := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			Status:     eventv1.EventConfigStatus{EventStore: &esRef},
		}
		setReady(&ec)

		es := &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "default"},
		}
		setReady(es)

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "es-1", Namespace: "default"}, &pubsubv1.EventStore{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.EventStore) = *es
			}).
			Return(nil)

		result, err := util.GetEventStoreForZone(ctx, "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("es-1"))
	})

	It("should propagate error when EventConfig lookup fails", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Return(fmt.Errorf("list failed"))

		result, err := util.GetEventStoreForZone(ctx, "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("list failed"))
	})

	It("should return BlockedError when EventStore ref is nil", func() {
		ec := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			Status:     eventv1.EventConfigStatus{EventStore: nil},
		}
		setReady(&ec)

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil)

		result, err := util.GetEventStoreForZone(ctx, "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("no EventStore reference"))
	})

	It("should return BlockedError when EventStore is not found", func() {
		esRef := ctypes.ObjectRef{Name: "es-missing", Namespace: "default"}
		ec := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			Status:     eventv1.EventConfigStatus{EventStore: &esRef},
		}
		setReady(&ec)

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "es-missing", Namespace: "default"}, &pubsubv1.EventStore{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "eventstores"}, "es-missing"))

		result, err := util.GetEventStoreForZone(ctx, "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return BlockedError when EventStore is not ready", func() {
		esRef := ctypes.ObjectRef{Name: "es-1", Namespace: "default"}
		ec := eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			Status:     eventv1.EventConfigStatus{EventStore: &esRef},
		}
		setReady(&ec)

		es := &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "default"},
		}
		// Not ready — no condition set

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventConfigList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "es-1", Namespace: "default"}, &pubsubv1.EventStore{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.EventStore) = *es
			}).
			Return(nil)

		result, err := util.GetEventStoreForZone(ctx, "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- FindActiveEventType ----------

var _ = Describe("FindActiveEventType", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return error on List failure", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventTypeList{}).
			Return(fmt.Errorf("timeout"))

		found, result, err := util.FindActiveEventType(ctx, "de.telekom.test.v1")
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list EventTypes"))
	})

	It("should return false when no matching type found", func() {
		items := []eventv1.EventType{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "other-type"},
				Spec:       eventv1.EventTypeSpec{Type: "de.telekom.other.v1"},
				Status:     eventv1.EventTypeStatus{Active: true},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventTypeList{}).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{Items: items}
			}).
			Return(nil)

		found, result, err := util.FindActiveEventType(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(result).To(BeNil())
	})

	It("should return false when matching type exists but none are active", func() {
		items := []eventv1.EventType{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test-type"},
				Spec:       eventv1.EventTypeSpec{Type: "de.telekom.test.v1"},
				Status:     eventv1.EventTypeStatus{Active: false},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventTypeList{}).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{Items: items}
			}).
			Return(nil)

		found, result, err := util.FindActiveEventType(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(result).To(BeNil())
	})

	It("should return active EventType when found and ready", func() {
		et := eventv1.EventType{
			ObjectMeta: metav1.ObjectMeta{Name: "test-type", CreationTimestamp: metav1.Now()},
			Spec:       eventv1.EventTypeSpec{Type: "de.telekom.test.v1"},
			Status:     eventv1.EventTypeStatus{Active: true},
		}
		setReady(&et)

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventTypeList{}).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{Items: []eventv1.EventType{et}}
			}).
			Return(nil)

		found, result, err := util.FindActiveEventType(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("test-type"))
	})

	It("should return BlockedError when active EventType is not ready", func() {
		et := eventv1.EventType{
			ObjectMeta: metav1.ObjectMeta{Name: "test-type", CreationTimestamp: metav1.Now()},
			Spec:       eventv1.EventTypeSpec{Type: "de.telekom.test.v1"},
			Status:     eventv1.EventTypeStatus{Active: true},
		}
		// Not ready

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventTypeList{}).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{Items: []eventv1.EventType{et}}
			}).
			Return(nil)

		found, result, err := util.FindActiveEventType(ctx, "de.telekom.test.v1")
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(result).ToNot(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- GetApplication ----------

var _ = Describe("GetApplication", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		objRef     ctypes.ObjectRef
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		objRef = ctypes.ObjectRef{Name: "my-app", Namespace: "default"}
	})

	It("should return application when found and ready", func() {
		expected := &applicationapi.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
		}
		setReady(expected)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "my-app", Namespace: "default"}, &applicationapi.Application{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationapi.Application) = *expected
			}).
			Return(nil)

		result, err := util.GetApplication(ctx, objRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("my-app"))
	})

	It("should return BlockedError when application is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "my-app", Namespace: "default"}, &applicationapi.Application{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "application.cp.ei.telekom.de", Resource: "applications"}, "my-app"))

		result, err := util.GetApplication(ctx, objRef)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return wrapped error on unexpected Get failure", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "my-app", Namespace: "default"}, &applicationapi.Application{}).
			Return(fmt.Errorf("connection refused"))

		result, err := util.GetApplication(ctx, objRef)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get application"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return BlockedError when application is not ready", func() {
		notReady := &applicationapi.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
		}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "my-app", Namespace: "default"}, &applicationapi.Application{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationapi.Application) = *notReady
			}).
			Return(nil)

		result, err := util.GetApplication(ctx, objRef)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- FindCrossZoneSSESubscriptionZones ----------

var _ = Describe("FindCrossZoneSSESubscriptionZones", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return error on List failure", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Return(fmt.Errorf("list error"))

		result, err := util.FindCrossZoneSSESubscriptionZones(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list EventSubscriptions"))
	})

	It("should return empty when no matching subscriptions", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventSubscriptionList) = eventv1.EventSubscriptionList{Items: []eventv1.EventSubscription{}}
			}).
			Return(nil)

		result, err := util.FindCrossZoneSSESubscriptionZones(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeEmpty())
	})

	It("should filter correctly: skip wrong type, non-SSE, same-zone, unapproved", func() {
		// Build subscriptions covering each skip condition
		subs := []eventv1.EventSubscription{
			// Wrong event type → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-wrong-type"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.other.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
			},
			// Non-SSE delivery → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-callback"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeCallback},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
			},
			// Same zone → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-same-zone"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
					Zone:      ctypes.ObjectRef{Name: "zone-a", Namespace: "default"},
				},
			},
			// Unapproved (non-nil condition with Status=False) → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-unapproved"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
					Zone:      ctypes.ObjectRef{Name: "zone-c", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "ApprovalGranted",
							Status: metav1.ConditionFalse,
							Reason: "Denied",
						},
					},
				},
			},
			// Valid cross-zone SSE approved → should be included
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-valid"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "ApprovalGranted",
							Status: metav1.ConditionTrue,
							Reason: "Approved",
						},
					},
				},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventSubscriptionList) = eventv1.EventSubscriptionList{Items: subs}
			}).
			Return(nil)

		result, err := util.FindCrossZoneSSESubscriptionZones(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Name).To(Equal("zone-b"))
	})

	It("should deduplicate zones", func() {
		subs := []eventv1.EventSubscription{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-1"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{Type: "ApprovalGranted", Status: metav1.ConditionTrue, Reason: "Approved"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-2"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{Type: "ApprovalGranted", Status: metav1.ConditionTrue, Reason: "Approved"},
					},
				},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventSubscriptionList) = eventv1.EventSubscriptionList{Items: subs}
			}).
			Return(nil)

		result, err := util.FindCrossZoneSSESubscriptionZones(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Name).To(Equal("zone-b"))
	})
})

// ---------- AnyOtherEventExposureExists ----------

var _ = Describe("AnyOtherEventExposureExists", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return error when FindEventExposures fails", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventExposureList{}, mock.Anything).
			Return(fmt.Errorf("list error"))

		found, err := util.AnyOtherEventExposureExists(ctx, "de.telekom.test.v1", "uid-1")
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeFalse())
	})

	It("should return false when no other exposure exists", func() {
		items := []eventv1.EventExposure{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-self", Namespace: "default", UID: "uid-1"},
				Spec:       eventv1.EventExposureSpec{EventType: "de.telekom.test.v1"},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventExposureList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventExposureList) = eventv1.EventExposureList{Items: items}
			}).
			Return(nil)

		found, err := util.AnyOtherEventExposureExists(ctx, "de.telekom.test.v1", "uid-1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
	})

	It("should return true when another exposure exists", func() {
		items := []eventv1.EventExposure{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-self", Namespace: "default", UID: "uid-1"},
				Spec:       eventv1.EventExposureSpec{EventType: "de.telekom.test.v1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-other", Namespace: "default", UID: "uid-2"},
				Spec:       eventv1.EventExposureSpec{EventType: "de.telekom.test.v1"},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventExposureList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventExposureList) = eventv1.EventExposureList{Items: items}
			}).
			Return(nil)

		found, err := util.AnyOtherEventExposureExists(ctx, "de.telekom.test.v1", "uid-1")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
	})
})

// ---------- FindEventExposures ----------

var _ = Describe("FindEventExposures", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return error on List failure", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventExposureList{}, mock.Anything).
			Return(fmt.Errorf("api error"))

		result, err := util.FindEventExposures(ctx, "de.telekom.test.v1")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list EventExposures"))
	})

	It("should return empty slice when no matches", func() {
		items := []eventv1.EventExposure{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-other"},
				Spec:       eventv1.EventExposureSpec{EventType: "de.telekom.other.v1"},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventExposureList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventExposureList) = eventv1.EventExposureList{Items: items}
			}).
			Return(nil)

		result, err := util.FindEventExposures(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeEmpty())
	})

	It("should return matching exposures filtered by eventType", func() {
		items := []eventv1.EventExposure{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-match"},
				Spec:       eventv1.EventExposureSpec{EventType: "de.telekom.test.v1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-other"},
				Spec:       eventv1.EventExposureSpec{EventType: "de.telekom.other.v1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-match-2"},
				Spec:       eventv1.EventExposureSpec{EventType: "de.telekom.test.v1"},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventExposureList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventExposureList) = eventv1.EventExposureList{Items: items}
			}).
			Return(nil)

		result, err := util.FindEventExposures(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(HaveLen(2))
		Expect(result[0].Name).To(Equal("exp-match"))
		Expect(result[1].Name).To(Equal("exp-match-2"))
	})
})

// ---------- FindActiveEventExposure (pure function) ----------

var _ = Describe("FindActiveEventExposure", func() {
	It("should return false when exposure list is empty", func() {
		found, result, err := util.FindActiveEventExposure([]eventv1.EventExposure{})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(result).To(BeNil())
	})

	It("should return false when none are active", func() {
		exposures := []eventv1.EventExposure{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-1"},
				Status:     eventv1.EventExposureStatus{Active: false},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-2"},
				Status:     eventv1.EventExposureStatus{Active: false},
			},
		}

		found, result, err := util.FindActiveEventExposure(exposures)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(result).To(BeNil())
	})

	It("should return active exposure when found and ready", func() {
		exp := eventv1.EventExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "exp-active", CreationTimestamp: metav1.Now()},
			Status:     eventv1.EventExposureStatus{Active: true},
		}
		setReady(&exp)

		found, result, err := util.FindActiveEventExposure([]eventv1.EventExposure{exp})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("exp-active"))
	})

	It("should pick the oldest active exposure by creation time", func() {
		now := metav1.Now()
		later := metav1.NewTime(now.Add(time.Minute))

		expOld := eventv1.EventExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "exp-old", CreationTimestamp: now},
			Status:     eventv1.EventExposureStatus{Active: true},
		}
		setReady(&expOld)

		expNew := eventv1.EventExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "exp-new", CreationTimestamp: later},
			Status:     eventv1.EventExposureStatus{Active: true},
		}
		setReady(&expNew)

		// Pass new first to ensure sorting works
		found, result, err := util.FindActiveEventExposure([]eventv1.EventExposure{expNew, expOld})
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("exp-old"))
	})

	It("should return BlockedError when active exposure is not ready", func() {
		exp := eventv1.EventExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "exp-active", CreationTimestamp: metav1.Now()},
			Status:     eventv1.EventExposureStatus{Active: true},
		}
		// Not ready — no condition set

		found, result, err := util.FindActiveEventExposure([]eventv1.EventExposure{exp})
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(result).ToNot(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})
})

// ---------- FindCrossZoneCallbackSubscriptions ----------

var _ = Describe("FindCrossZoneCallbackSubscriptions", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return error on List failure", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Return(fmt.Errorf("list error"))

		result, err := util.FindCrossZoneCallbackSubscriptions(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to list EventSubscriptions"))
	})

	It("should return empty when no matching subscriptions", func() {
		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventSubscriptionList) = eventv1.EventSubscriptionList{Items: []eventv1.EventSubscription{}}
			}).
			Return(nil)

		result, err := util.FindCrossZoneCallbackSubscriptions(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeEmpty())
	})

	It("should filter correctly: skip wrong type, non-callback, same-zone, unapproved", func() {
		subs := []eventv1.EventSubscription{
			// Wrong event type → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-wrong-type"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.other.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeCallback},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
			},
			// Non-callback delivery (SSE) → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-sse"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeServerSentEvent},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
			},
			// Same zone → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-same-zone"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeCallback},
					Zone:      ctypes.ObjectRef{Name: "zone-a", Namespace: "default"},
				},
			},
			// Unapproved (non-nil condition with Status=False) → skip
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-unapproved"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeCallback},
					Zone:      ctypes.ObjectRef{Name: "zone-c", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "ApprovalGranted",
							Status: metav1.ConditionFalse,
							Reason: "Denied",
						},
					},
				},
			},
			// Valid cross-zone callback approved → should be included
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-valid"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeCallback},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "ApprovalGranted",
							Status: metav1.ConditionTrue,
							Reason: "Approved",
						},
					},
				},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventSubscriptionList) = eventv1.EventSubscriptionList{Items: subs}
			}).
			Return(nil)

		result, err := util.FindCrossZoneCallbackSubscriptions(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Name).To(Equal("sub-valid"))
	})

	It("should return multiple matching subscriptions", func() {
		subs := []eventv1.EventSubscription{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-1"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeCallback},
					Zone:      ctypes.ObjectRef{Name: "zone-b", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{Type: "ApprovalGranted", Status: metav1.ConditionTrue, Reason: "Approved"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-2"},
				Spec: eventv1.EventSubscriptionSpec{
					EventType: "de.telekom.test.v1",
					Delivery:  eventv1.Delivery{Type: eventv1.DeliveryTypeCallback},
					Zone:      ctypes.ObjectRef{Name: "zone-c", Namespace: "default"},
				},
				Status: eventv1.EventSubscriptionStatus{
					Conditions: []metav1.Condition{
						{Type: "ApprovalGranted", Status: metav1.ConditionTrue, Reason: "Approved"},
					},
				},
			},
		}

		fakeClient.EXPECT().
			List(ctx, &eventv1.EventSubscriptionList{}, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventSubscriptionList) = eventv1.EventSubscriptionList{Items: subs}
			}).
			Return(nil)

		result, err := util.FindCrossZoneCallbackSubscriptions(ctx, "de.telekom.test.v1", "zone-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(HaveLen(2))
		Expect(result[0].Name).To(Equal("sub-1"))
		Expect(result[1].Name).To(Equal("sub-2"))
	})
})
