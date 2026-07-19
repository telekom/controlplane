// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticserver_test

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
	"github.com/telekom/controlplane/agentic/internal/handler/agenticserver"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newAgenticServer(name, basePath string, uid types.UID, creationTime time.Time) *agenticv1.AgenticServer {
	return &agenticv1.AgenticServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               uid,
			CreationTimestamp: metav1.NewTime(creationTime),
			Labels: map[string]string{
				agenticv1.AgenticBasePathLabelKey: basePath,
			},
		},
		Spec: agenticv1.AgenticServerSpec{
			BasePath: basePath,
			Version:  "1.0.0",
			Name:     "Test MCP Server",
		},
	}
}

var _ = Describe("AgenticServerHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *agenticserver.AgenticServerHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &agenticserver.AgenticServerHandler{}
	})

	Describe("CreateOrUpdate", func() {
		It("should return an error when List fails", func() {
			obj := newAgenticServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgenticServerList"), mock.Anything).
				Return(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list AgenticServers"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("should set Active=true when no other AgenticServer exists for basePath", func() {
			obj := newAgenticServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgenticServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgenticServerList) = agenticv1.AgenticServerList{
						Items: []agenticv1.AgenticServer{*obj},
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

		It("should set Active=false when another older AgenticServer exists for same basePath", func() {
			now := time.Now()
			existing := newAgenticServer("mcp-existing", "/mcp/weather/v1", "uid-existing", now.Add(-time.Hour))
			obj := newAgenticServer("mcp-new", "/mcp/weather/v1", "uid-new", now)

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgenticServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgenticServerList) = agenticv1.AgenticServerList{
						Items: []agenticv1.AgenticServer{*existing, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.Status.Conditions, condition.ConditionTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("AgenticServerNotActive"))
		})

		It("should set Active=true when it is the oldest AgenticServer for basePath", func() {
			now := time.Now()
			obj := newAgenticServer("mcp-oldest", "/mcp/weather/v1", "uid-oldest", now.Add(-time.Hour))
			newer := newAgenticServer("mcp-newer", "/mcp/weather/v1", "uid-newer", now)

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgenticServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgenticServerList) = agenticv1.AgenticServerList{
						Items: []agenticv1.AgenticServer{*newer, *obj},
					}
				}).
				Return(nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
		})

		It("should ignore AgenticServers with different basePaths", func() {
			now := time.Now()
			obj := newAgenticServer("mcp-1", "/mcp/weather/v1", "uid-1", now)
			different := newAgenticServer("mcp-other", "/mcp/other/v1", "uid-other", now.Add(-time.Hour))
			different.Spec.BasePath = "/mcp/other/v1"

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgenticServerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgenticServerList) = agenticv1.AgenticServerList{
						Items: []agenticv1.AgenticServer{*different, *obj},
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
			obj := newAgenticServer("mcp-1", "/mcp/weather/v1", "uid-1", time.Now())

			err := h.Delete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
