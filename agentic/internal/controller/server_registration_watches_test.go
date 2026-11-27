// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("server registration watches", func() {
	It("maps changes to registrations with the same normalized basePath", func() {
		scheme := runtime.NewScheme()
		Expect(agenticv1.AddToScheme(scheme)).To(Succeed())

		const path = "/shared/path/v1"
		const environment = "test"
		mcpServer := &agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mcp",
				Namespace: "default",
				UID:       "mcp",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: environment,
					agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
				},
			},
			Spec: agenticv1.McpServerSpec{BasePath: path},
		}
		otherMcpServer := &agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-mcp",
				Namespace: "default",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: environment,
					agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
				},
			},
			Spec: agenticv1.McpServerSpec{BasePath: "/Shared/Path/V1"},
		}
		peerMcpServer := &agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "peer-mcp",
				Namespace: "default",
				UID:       "peer-mcp",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: environment,
					agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
				},
			},
			Spec: agenticv1.McpServerSpec{BasePath: path},
		}
		foreignMcpServer := &agenticv1.McpServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foreign-mcp",
				Namespace: "default",
				UID:       "foreign-mcp",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: "other",
					agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
				},
			},
			Spec: agenticv1.McpServerSpec{BasePath: path},
		}
		agentCard := &agenticv1.AgentCard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent",
				Namespace: "default",
				UID:       "agent",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: environment,
					agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
				},
			},
			Spec: agenticv1.AgentCardSpec{BasePath: path},
		}
		peerAgentCard := &agenticv1.AgentCard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "peer-agent",
				Namespace: "default",
				UID:       "peer-agent",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: environment,
					agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
				},
			},
			Spec: agenticv1.AgentCardSpec{BasePath: path},
		}
		foreignAgentCard := &agenticv1.AgentCard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foreign-agent",
				Namespace: "default",
				UID:       "foreign-agent",
				Labels: map[string]string{
					cconfig.EnvironmentLabelKey: "other",
					agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
				},
			},
			Spec: agenticv1.AgentCardSpec{BasePath: path},
		}
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(mcpServer, otherMcpServer, peerMcpServer, foreignMcpServer, agentCard, peerAgentCard, foreignAgentCard).
			Build()

		mcpReconciler := &McpServerReconciler{Client: k8sClient}
		mcpReqs := mcpReconciler.MapAgentCardToMcpServer(context.Background(), agentCard)
		Expect(mcpReqs).To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(mcpServer)}, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(otherMcpServer)}, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(peerMcpServer)}))

		agentReconciler := &AgentCardReconciler{Client: k8sClient}
		agentReqs := agentReconciler.MapMcpServerToAgentCard(context.Background(), mcpServer)
		Expect(agentReqs).To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(agentCard)}, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(peerAgentCard)}))

		Expect(mcpReconciler.MapMcpServerToMcpServer(context.Background(), mcpServer)).
			To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(otherMcpServer)}, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(peerMcpServer)}))
		Expect(agentReconciler.MapAgentCardToAgentCard(context.Background(), agentCard)).
			To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(peerAgentCard)}))
	})

	It("maps both old and new basePaths when a registration changes", func() {
		var mappedPaths []string
		mapFunc := func(_ context.Context, obj client.Object) []reconcile.Request {
			switch registration := obj.(type) {
			case *agenticv1.McpServer:
				mappedPaths = append(mappedPaths, registration.Spec.BasePath)
			case *agenticv1.AgentCard:
				mappedPaths = append(mappedPaths, registration.Spec.BasePath)
			}
			return nil
		}
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		defer queue.ShutDown()

		old := &agenticv1.McpServer{Spec: agenticv1.McpServerSpec{BasePath: "/old/path"}}
		current := &agenticv1.McpServer{Spec: agenticv1.McpServerSpec{BasePath: "/new/path"}}
		enqueueMappedObjects(mapFunc).Update(context.Background(), event.UpdateEvent{
			ObjectOld: old,
			ObjectNew: current,
		}, queue)

		Expect(mappedPaths).To(Equal([]string{"/old/path", "/new/path"}))
	})
})
