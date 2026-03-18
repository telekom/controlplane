// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/handler/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

var _ = Describe("GetPublisher", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		objRef     ctypes.ObjectRef
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		objRef = ctypes.ObjectRef{Name: "test-publisher", Namespace: "default"}
	})

	It("should return publisher when found and ready", func() {
		expected := &pubsubv1.Publisher{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-publisher",
				Namespace: "default",
			},
		}
		meta.SetStatusCondition(&expected.Status.Conditions, metav1.Condition{
			Type:   condition.ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})

		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, &pubsubv1.Publisher{}).
			Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.Publisher) = *expected
			}).
			Return(nil)

		result, err := util.GetPublisher(ctx, objRef)

		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("test-publisher"))
	})

	It("should return BlockedError when publisher is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, &pubsubv1.Publisher{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "publishers"}, "test-publisher"))

		result, err := util.GetPublisher(ctx, objRef)

		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return BlockedError when publisher is not ready", func() {
		notReady := &pubsubv1.Publisher{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-publisher",
				Namespace: "default",
			},
		}
		// No Ready condition set

		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, &pubsubv1.Publisher{}).
			Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.Publisher) = *notReady
			}).
			Return(nil)

		result, err := util.GetPublisher(ctx, objRef)

		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should return wrapped error on unexpected Get failure", func() {
		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-publisher", Namespace: "default"}, &pubsubv1.Publisher{}).
			Return(fmt.Errorf("connection refused"))

		result, err := util.GetPublisher(ctx, objRef)

		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
		Expect(err.Error()).To(ContainSubstring("failed to get Publisher"))
	})
})

var _ = Describe("GetEventStore", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		objRef     ctypes.ObjectRef
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		objRef = ctypes.ObjectRef{Name: "test-eventstore", Namespace: "default"}
	})

	It("should return eventstore when found and ready", func() {
		expected := &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-eventstore",
				Namespace: "default",
			},
		}
		meta.SetStatusCondition(&expected.Status.Conditions, metav1.Condition{
			Type:   condition.ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		})

		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
			Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.EventStore) = *expected
			}).
			Return(nil)

		result, err := util.GetEventStore(ctx, objRef)

		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Name).To(Equal("test-eventstore"))
	})

	It("should return BlockedError when eventstore is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "eventstores"}, "test-eventstore"))

		result, err := util.GetEventStore(ctx, objRef)

		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return BlockedError when eventstore is not ready", func() {
		notReady := &pubsubv1.EventStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-eventstore",
				Namespace: "default",
			},
		}

		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
			Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.EventStore) = *notReady
			}).
			Return(nil)

		result, err := util.GetEventStore(ctx, objRef)

		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should return wrapped error on unexpected Get failure", func() {
		fakeClient.EXPECT().
			Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
			Return(fmt.Errorf("connection refused"))

		result, err := util.GetEventStore(ctx, objRef)

		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
		Expect(err.Error()).To(ContainSubstring("failed to get EventStore"))
	})
})
