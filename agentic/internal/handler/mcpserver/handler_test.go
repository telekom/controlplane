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
				agenticv1.McpBasePathLabelKey: basePath,
			},
		},
		Spec: agenticv1.McpServerSpec{
			BasePath: basePath,
			Version:  "1.0.0",
			Name:     "Test MCP Server",
		},
	}
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
			obj := newMcpServer("mcp-new", "/mcp/weather/v1", "uid-new", now)

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
			Expect(readyCond.Reason).To(Equal("McpServerAlreadyExists"))
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

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
		})

		It("should ignore McpServers with different basePaths", func() {
			now := time.Now()
			obj := newMcpServer("mcp-1", "/mcp/weather/v1", "uid-1", now)
			different := newMcpServer("mcp-other", "/mcp/other/v1", "uid-other", now.Add(-time.Hour))
			different.Spec.BasePath = "/mcp/other/v1"

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{
						Items: []agenticv1.McpServer{*different, *obj},
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
			obj := newMcpServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			err := h.Delete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
