// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver_test

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
	"github.com/telekom/controlplane/agentic/internal/handler/mcpserver"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newMcpServer(name, basePath string, uid types.UID, creationTime time.Time) *agenticv1.McpServer {
	return &agenticv1.McpServer{
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
		Spec: agenticv1.McpServerSpec{
			BasePath: basePath,
			Version:  "1.0.0",
			Name:     "Test MCP Server",
		},
	}
}

func mockListAgentCards(fakeClient *fakeclient.MockJanitorClient, ctx context.Context, items ...agenticv1.AgentCard) {
	fakeClient.EXPECT().
		List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{Items: items}
		}).
		Return(nil)
}

var _ = Describe("McpServerHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *mcpserver.McpServerHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &mcpserver.McpServerHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("should return an error when List fails", func() {
			obj := newMcpServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Return(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list McpServers"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("should scope McpServer candidates to the object's environment", func() {
			obj := newMcpServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.MatchedBy(func(option client.ListOption) bool {
					matchingLabels, ok := option.(client.MatchingLabels)
					return ok && matchingLabels[cconfig.EnvironmentLabelKey] == "default"
				})).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())
		})

		It("should set Active=true when no other McpServer exists for basePath", func() {
			obj := newMcpServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{
						Items: []agenticv1.McpServer{*obj},
					}
				}).
				Return(nil)
			mockListAgentCards(fakeClient, ctx)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())

			readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set Active=false when another older McpServer exists for same basePath", func() {
			now := time.Now()
			existing := newMcpServer("mcp-existing", "/mcp/weather/v1", "uid-existing", now.Add(-time.Hour))
			existing.Namespace = "prod--platform--mcp-team"
			obj := newMcpServer("mcp-new", "/mcp/weather/v1", "uid-new", now)
			obj.Namespace = "prod--platform--consumer-team"

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{
						Items: []agenticv1.McpServer{*existing, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(condition.ReasonPreconditionNotMet))
			Expect(readyCond.Message).To(Equal(
				`Another McpServer with base path "/mcp/weather/v1" already exists and prevents this instance from becoming active. It is owned by team "platform--mcp-team". ` +
					condition.MessageWaitingForTakeover,
			))

			processingCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeProcessing)
			Expect(processingCond).NotTo(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal(condition.ReasonBlocked))
			Expect(processingCond.Message).To(Equal(readyCond.Message))
		})

		It("should set Active=true when it is the oldest McpServer for basePath", func() {
			now := time.Now()
			obj := newMcpServer("mcp-oldest", "/mcp/weather/v1", "uid-oldest", now.Add(-time.Hour))
			newer := newMcpServer("mcp-newer", "/mcp/weather/v1", "uid-newer", now)

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{
						Items: []agenticv1.McpServer{*newer, *obj},
					}
				}).
				Return(nil)
			mockListAgentCards(fakeClient, ctx)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
		})

		It("should be blocked by an older AgentCard with the same basePath", func() {
			now := time.Now()
			obj := newMcpServer("mcp-1", "/shared/path/v1", "mcp-uid", now)
			olderCard := agenticv1.AgentCard{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "agent-1",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(now.Add(-time.Hour)),
				},
				Spec: agenticv1.AgentCardSpec{BasePath: obj.Spec.BasePath},
			}

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{Items: []agenticv1.McpServer{*obj}}
				}).
				Return(nil)
			mockListAgentCards(fakeClient, ctx, olderCard)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Message).To(ContainSubstring("Another AgentCard"))
			Expect(readyCond.Message).To(ContainSubstring(obj.Spec.BasePath))
		})

		It("should use kind as a deterministic final tie-breaker", func() {
			now := time.Now()
			obj := newMcpServer("same-name", "/shared/path/v1", "mcp-uid", now)
			obj.Namespace = "same-namespace"
			card := agenticv1.AgentCard{
				ObjectMeta: metav1.ObjectMeta{
					Name:              obj.Name,
					Namespace:         obj.Namespace,
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: agenticv1.AgentCardSpec{BasePath: obj.Spec.BasePath},
			}

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{Items: []agenticv1.McpServer{*obj}}
				}).
				Return(nil)
			mockListAgentCards(fakeClient, ctx, card)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())
			Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady).Message).
				To(ContainSubstring("Another AgentCard"))
		})

		It("should not treat a different exact basePath from an AgentCard as a conflict", func() {
			now := time.Now()
			obj := newMcpServer("mcp-1", "/shared/path/v1", "mcp-uid", now)
			caseDifferentCard := agenticv1.AgentCard{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "agent-1",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(now.Add(-time.Hour)),
				},
				Spec: agenticv1.AgentCardSpec{BasePath: "/Shared/Path/V1"},
			}

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{Items: []agenticv1.McpServer{*obj}}
				}).
				Return(nil)
			mockListAgentCards(fakeClient, ctx, caseDifferentCard)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
		})

		It("should treat McpServers with different basePaths as case conflicts", func() {
			now := time.Now()
			obj := newMcpServer("mcp-1", "/mcp/weather/v1", "uid-1", now)
			caseConflict := newMcpServer("mcp-other", "/Mcp/Weather/V1", "uid-other", now.Add(-time.Hour))

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{
						Items: []agenticv1.McpServer{*caseConflict, *obj},
					}
				}).
				Return(nil)

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
			obj := newMcpServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			err := h.Delete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
