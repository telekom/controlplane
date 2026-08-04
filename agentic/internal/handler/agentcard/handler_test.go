// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard_test

import (
	"context"
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/agentcard"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newAgentCard(name, basePath string, uid types.UID, creationTime time.Time) *agenticv1.AgentCard {
	return &agenticv1.AgentCard{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               uid,
			CreationTimestamp: metav1.NewTime(creationTime),
			Labels: map[string]string{
				agenticv1.AgentBasePathLabelKey: basePath,
			},
		},
		Spec: agenticv1.AgentCardSpec{
			BasePath: basePath,
			Version:  "1.0.0",
			Name:     "Test Agent",
		},
	}
}

var _ = Describe("AgentCardHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *agentcard.AgentCardHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &agentcard.AgentCardHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("should return an error when List fails", func() {
			obj := newAgentCard("agent-1", "/agent/assistant/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Return(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list AgentCards"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("should set Active=true when no other AgentCard exists for basePath", func() {
			obj := newAgentCard("agent-1", "/agent/assistant/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{
						Items: []agenticv1.AgentCard{*obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())

			readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set Active=false when another older AgentCard exists for same basePath", func() {
			now := time.Now()
			existing := newAgentCard("agent-existing", "/agent/assistant/v1", "uid-existing", now.Add(-time.Hour))
			obj := newAgentCard("agent-new", "/agent/assistant/v1", "uid-new", now)

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{
						Items: []agenticv1.AgentCard{*existing, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("AgentCardNotActive"))
		})

		It("should set Active=true when it is the oldest AgentCard for basePath", func() {
			now := time.Now()
			obj := newAgentCard("agent-oldest", "/agent/assistant/v1", "uid-oldest", now.Add(-time.Hour))
			newer := newAgentCard("agent-newer", "/agent/assistant/v1", "uid-newer", now)

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{
						Items: []agenticv1.AgentCard{*newer, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
		})

		It("should ignore AgentCards with different basePaths", func() {
			now := time.Now()
			obj := newAgentCard("agent-1", "/agent/assistant/v1", "uid-1", now)
			different := newAgentCard("agent-other", "/agent/other/v1", "uid-other", now.Add(-time.Hour))
			different.Spec.BasePath = "/agent/other/v1"

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{
						Items: []agenticv1.AgentCard{*different, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should succeed without errors", func() {
			obj := newAgentCard("agent-1", "/agent/assistant/v1", "uid-1", time.Now())

			err := h.Delete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
