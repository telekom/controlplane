// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package publisher_test

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/handler/publisher"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newPublisher() *pubsubv1.Publisher {
	return &pubsubv1.Publisher{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-publisher",
			Namespace: "default",
		},
		Spec: pubsubv1.PublisherSpec{
			EventStore: ctypes.ObjectRef{
				Name:      "test-eventstore",
				Namespace: "default",
			},
			EventType:   "de.telekom.test.event.v1",
			PublisherId: "test-app",
		},
	}
}

func readyEventStore() *pubsubv1.EventStore {
	es := &pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-eventstore",
			Namespace: "default",
		},
		Spec: pubsubv1.EventStoreSpec{
			Url:          "https://config-server.example.com",
			TokenUrl:     "https://auth.example.com/token",
			ClientId:     "client-id",
			ClientSecret: "client-secret",
		},
	}
	meta.SetStatusCondition(&es.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return es
}

var _ = Describe("PublisherHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		handler    *publisher.PublisherHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		handler = &publisher.PublisherHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("should set Ready and DoneProcessing when EventStore exists and is ready", func() {
			obj := newPublisher()
			es := readyEventStore()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *es
				}).
				Return(nil)

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond.Reason).To(Equal("PublisherReady"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})

		It("should return BlockedError when EventStore is not found", func() {
			obj := newPublisher()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "eventstores"}, "test-eventstore"))

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err).To(BeAssignableToTypeOf(&ctrlerrors.CtrlError{}))
			rootCause := unwrapAll(err)
			var blockedErr ctrlerrors.BlockedError
			Expect(errors.As(rootCause, &blockedErr)).To(BeTrue())
		})

		It("should return BlockedError when EventStore is not ready", func() {
			obj := newPublisher()
			es := &pubsubv1.EventStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-eventstore",
					Namespace: "default",
				},
			}
			// EventStore without Ready condition

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
				Run(func(_ context.Context, _ types.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*pubsubv1.EventStore) = *es
				}).
				Return(nil)

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			rootCause := unwrapAll(err)
			Expect(rootCause).To(Satisfy(isBlockedError))
		})

		It("should return error when Get fails with unexpected error", func() {
			obj := newPublisher()

			fakeClient.EXPECT().
				Get(ctx, types.NamespacedName{Name: "test-eventstore", Namespace: "default"}, &pubsubv1.EventStore{}).
				Return(fmt.Errorf("connection refused"))

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})
	})

	Describe("Delete", func() {
		It("should return nil", func() {
			obj := newPublisher()

			err := handler.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})
	})
})

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
	var be ctrlerrors.BlockedError
	return errors.As(err, &be) && be.IsBlocked()
}
