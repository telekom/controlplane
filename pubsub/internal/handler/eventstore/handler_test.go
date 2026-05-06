// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventstore_test

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/handler/eventstore"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventStoreHandler", func() {
	var (
		ctx     context.Context
		handler *eventstore.EventStoreHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		handler = &eventstore.EventStoreHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("should set Ready and DoneProcessing conditions", func() {
			obj := &pubsubv1.EventStore{
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

			err := handler.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(meta.IsStatusConditionTrue(obj.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond.Reason).To(Equal("EventStoreReady"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})
	})

	Describe("Delete", func() {
		It("should return nil without errors", func() {
			obj := &pubsubv1.EventStore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-eventstore",
					Namespace: "default",
				},
			}

			err := handler.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})
	})
})
