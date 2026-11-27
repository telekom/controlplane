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
	cconfig "github.com/telekom/controlplane/common/pkg/config"

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
				cconfig.EnvironmentLabelKey: "default",
				agenticv1.BasePathLabelKey:  basePath,
			},
		},
		Spec: agenticv1.AgentCardSpec{
			BasePath: basePath,
			Version:  "1.0.0",
			Name:     "Test Agent",
		},
	}
}

func mockListMcpServers(fakeClient *fakeclient.MockJanitorClient, ctx context.Context, items ...agenticv1.McpServer) {
	fakeClient.EXPECT().
		List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{Items: items}
		}).
		Return(nil)
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

		It("should scope AgentCard candidates to the object's environment", func() {
			obj := newAgentCard("agent-1", "/agent/assistant/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.MatchedBy(func(option client.ListOption) bool {
					matchingLabels, ok := option.(client.MatchingLabels)
					return ok && matchingLabels[cconfig.EnvironmentLabelKey] == "default"
				})).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())
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
			mockListMcpServers(fakeClient, ctx)

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
			existing.Namespace = "prod--platform--agent-team"
			obj := newAgentCard("agent-new", "/agent/assistant/v1", "uid-new", now)
			obj.Namespace = "prod--platform--consumer-team"

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{
						Items: []agenticv1.AgentCard{*existing, *obj},
					}
				}).
				Return(nil)
			mockListMcpServers(fakeClient, ctx)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(condition.ReasonPreconditionNotMet))
			Expect(readyCond.Message).To(Equal(
				`Another AgentCard with base path "/agent/assistant/v1" already exists and prevents this instance from becoming active. It is owned by team "platform--agent-team". ` +
					condition.MessageWaitingForTakeover,
			))

			processingCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeProcessing)
			Expect(processingCond).NotTo(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal(condition.ReasonBlocked))
			Expect(processingCond.Message).To(Equal(readyCond.Message))
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
			mockListMcpServers(fakeClient, ctx)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
		})

		It("should be blocked by an older McpServer with the same basePath", func() {
			now := time.Now()
			obj := newAgentCard("agent-1", "/shared/path/v1", "agent-uid", now)
			olderServer := agenticv1.McpServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mcp-1",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(now.Add(-time.Hour)),
				},
				Spec: agenticv1.McpServerSpec{BasePath: obj.Spec.BasePath},
			}

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{Items: []agenticv1.AgentCard{*obj}}
				}).
				Return(nil)
			mockListMcpServers(fakeClient, ctx, olderServer)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Message).To(ContainSubstring("Another McpServer"))
			Expect(readyCond.Message).To(ContainSubstring(obj.Spec.BasePath))
		})

		It("reports the oldest registration across both kinds", func() {
			now := time.Now()
			obj := newAgentCard("agent-new", "/shared/path", "agent-new", now)
			olderCard := newAgentCard("agent-old", "/shared/path", "agent-old", now.Add(-time.Hour))
			oldestServer := agenticv1.McpServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mcp",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
				},
				Spec: agenticv1.McpServerSpec{BasePath: "/shared/path"},
			}
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{Items: []agenticv1.AgentCard{*obj, *olderCard}}
				}).
				Return(nil)
			mockListMcpServers(fakeClient, ctx, oldestServer)

			Expect(h.CreateOrUpdate(ctx, obj)).To(Succeed())
			Expect(obj.Status.Active).To(BeFalse())
			processing := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processing.Message).To(ContainSubstring("McpServer"))
		})

		It("should treat AgentCards with different basePaths as case conflicts", func() {
			now := time.Now()
			obj := newAgentCard("agent-1", "/agent/assistant/v1", "uid-1", now)
			caseConflict := newAgentCard("agent-other", "/Agent/Assistant/V1", "uid-other", now.Add(-time.Hour))

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{
						Items: []agenticv1.AgentCard{*caseConflict, *obj},
					}
				}).
				Return(nil)
			mockListMcpServers(fakeClient, ctx)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Message).To(ContainSubstring("case conflict"))
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
