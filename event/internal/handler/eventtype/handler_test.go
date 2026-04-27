// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype_test

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventtype"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newEventType(name, specType string, uid types.UID, creationTime time.Time) *eventv1.EventType {
	return &eventv1.EventType{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               uid,
			CreationTimestamp: metav1.NewTime(creationTime),
		},
		Spec: eventv1.EventTypeSpec{
			Type:    specType,
			Version: "1.0.0",
		},
	}
}

var _ = Describe("EventTypeHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *eventtype.EventTypeHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &eventtype.EventTypeHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("should return an error wrapping 'failed to list EventTypes' when List fails", func() {
			obj := newEventType("et-1", "de.telekom.test.v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, &eventv1.EventTypeList{}).
				Return(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list EventTypes"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("should set Active=false with NotReady and Blocked conditions when no candidates match the type", func() {
			obj := newEventType("et-1", "de.telekom.test.v1", "uid-1", time.Now())

			// List returns items that do NOT match obj's type
			otherET := newEventType("et-other", "de.telekom.other.v1", "uid-other", time.Now())

			fakeClient.EXPECT().
				List(ctx, &eventv1.EventTypeList{}).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{
						Items: []eventv1.EventType{*otherET},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("EventTypeNotActive"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should set Active=true with Ready and DoneProcessing when this obj is the only candidate", func() {
			now := time.Now()
			obj := newEventType("et-1", "de.telekom.test.v1", "uid-1", now)

			fakeClient.EXPECT().
				List(ctx, &eventv1.EventTypeList{}).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{
						Items: []eventv1.EventType{*obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("EventTypeActive"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})

		It("should set Active=true when this obj is the oldest among multiple candidates", func() {
			t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			t3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

			obj := newEventType("et-oldest", "de.telekom.test.v1", "uid-oldest", t1)
			et2 := newEventType("et-middle", "de.telekom.test.v1", "uid-middle", t2)
			et3 := newEventType("et-newest", "de.telekom.test.v1", "uid-newest", t3)

			fakeClient.EXPECT().
				List(ctx, &eventv1.EventTypeList{}).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					// Return in arbitrary order; handler sorts by creation time
					*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{
						Items: []eventv1.EventType{*et3, *obj, *et2},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		})

		It("should set Active=false when this obj is NOT the oldest among multiple candidates", func() {
			t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

			older := newEventType("et-older", "de.telekom.test.v1", "uid-older", t1)
			obj := newEventType("et-newer", "de.telekom.test.v1", "uid-newer", t2)

			fakeClient.EXPECT().
				List(ctx, &eventv1.EventTypeList{}).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{
						Items: []eventv1.EventType{*obj, *older},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("EventTypeNotActive"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should skip deletion-marked candidates and activate the next non-deleted one", func() {
			t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			deletionTime := metav1.NewTime(time.Now())

			deletedET := newEventType("et-deleted", "de.telekom.test.v1", "uid-deleted", t1)
			deletedET.DeletionTimestamp = &deletionTime

			obj := newEventType("et-active", "de.telekom.test.v1", "uid-active", t2)

			fakeClient.EXPECT().
				List(ctx, &eventv1.EventTypeList{}).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{
						Items: []eventv1.EventType{*deletedET, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})

		It("should set Active=false when all candidates are deletion-marked", func() {
			t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			deletionTime := metav1.NewTime(time.Now())

			deleted1 := newEventType("et-deleted-1", "de.telekom.test.v1", "uid-deleted-1", t1)
			deleted1.DeletionTimestamp = &deletionTime

			obj := newEventType("et-deleted-2", "de.telekom.test.v1", "uid-deleted-2", t2)
			obj.DeletionTimestamp = &deletionTime

			fakeClient.EXPECT().
				List(ctx, &eventv1.EventTypeList{}).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{
						Items: []eventv1.EventType{*deleted1, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})
	})

	Describe("Delete", func() {
		It("should always return nil", func() {
			obj := newEventType("et-1", "de.telekom.test.v1", "uid-1", time.Now())

			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})
	})
})
