// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/agentcard"
	"github.com/telekom/controlplane/agentic/internal/handler/mcpserver"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func registrationMetadata(namespace, environment, name, path string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: name, Namespace: namespace,
		Labels: map[string]string{
			cconfig.EnvironmentLabelKey: environment,
			agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(path),
		},
	}
}

func createRegistrationPair(cardNamespace, serverNamespace, environment, cardName, serverName, cardPath, serverPath string) (*agenticv1.AgentCard, *agenticv1.McpServer) {
	card := &agenticv1.AgentCard{
		ObjectMeta: registrationMetadata(cardNamespace, environment, cardName, cardPath),
		Spec:       agenticv1.AgentCardSpec{BasePath: cardPath, Name: "Agent", Version: "1.0"},
	}
	server := &agenticv1.McpServer{
		ObjectMeta: registrationMetadata(serverNamespace, environment, serverName, serverPath),
		Spec:       agenticv1.McpServerSpec{BasePath: serverPath, Name: "MCP", Version: "1.0"},
	}
	Expect(k8sClient.Create(ctx, card)).To(Succeed())
	Expect(k8sClient.Create(ctx, server)).To(Succeed())
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(card), card)).To(Succeed())
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server)).To(Succeed())
	card.TypeMeta = metav1.TypeMeta{APIVersion: agenticv1.GroupVersion.String(), Kind: "AgentCard"}
	server.TypeMeta = metav1.TypeMeta{APIVersion: agenticv1.GroupVersion.String(), Kind: "McpServer"}
	return card, server
}

func expectRegistrationBlocked(g Gomega, conditions []metav1.Condition, kind, path string) {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	g.Expect(processing).NotTo(BeNil())
	g.Expect(processing.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(processing.Reason).To(Equal(condition.ReasonBlocked))
	g.Expect(processing.Message).To(ContainSubstring(kind))
	g.Expect(processing.Message).To(ContainSubstring(path))
}

var _ = Describe("server registration acceptance criteria", func() {
	var namespace, otherNamespace string

	BeforeEach(func() {
		first := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "registration-a-"}}
		second := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "registration-b-"}}
		Expect(k8sClient.Create(ctx, first)).To(Succeed())
		Expect(k8sClient.Create(ctx, second)).To(Succeed())
		namespace, otherNamespace = first.Name, second.Name
	})

	DescribeTable("evaluates both handlers consistently",
		func(cardName, serverName string, separateNamespaces bool, cardOffset, serverOffset time.Duration, cardWins bool) {
			cardNamespace, serverNamespace := namespace, namespace
			if separateNamespaces {
				serverNamespace = otherNamespace
			}
			const path = "/shared/registration"
			card, mcp := createRegistrationPair(cardNamespace, serverNamespace, namespace, cardName, serverName, path, path)
			cardTime, serverTime := card.CreationTimestamp, mcp.CreationTimestamp
			// Give each handler the intended relative timestamps while peer lists come from the API.
			card.CreationTimestamp = metav1.NewTime(serverTime.Add(cardOffset))
			mcp.CreationTimestamp = metav1.NewTime(cardTime.Add(serverOffset))
			handlerCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, namespace)))

			for range 2 {
				Expect((&agentcard.AgentCardHandler{}).CreateOrUpdate(handlerCtx, card)).To(Succeed())
				Expect((&mcpserver.McpServerHandler{}).CreateOrUpdate(handlerCtx, mcp)).To(Succeed())
				Expect(card.Status.Active).To(Equal(cardWins))
				Expect(mcp.Status.Active).To(Equal(!cardWins))
				if cardWins {
					expectRegistrationBlocked(Default, mcp.GetConditions(), "AgentCard", path)
				} else {
					expectRegistrationBlocked(Default, card.GetConditions(), "McpServer", path)
				}
			}
		},
		Entry("older AgentCard", "agent", "mcp", false, -time.Hour, time.Hour, true),
		Entry("older McpServer", "agent", "mcp", false, time.Hour, -time.Hour, false),
		Entry("equal timestamps ordered by namespace", "z-agent", "a-mcp", true, time.Duration(0), time.Duration(0), true),
		Entry("equal timestamps ordered by name", "z-agent", "a-mcp", false, time.Duration(0), time.Duration(0), false),
		Entry("equal timestamps, namespace and name ordered by kind", "same", "same", false, time.Duration(0), time.Duration(0), true),
	)

	It("does not block either kind for different normalized paths", func() {
		card, mcp := createRegistrationPair(namespace, namespace, namespace, "agent", "mcp", "/shared/path", "/shared/other")
		handlerCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, namespace)))

		Expect((&agentcard.AgentCardHandler{}).CreateOrUpdate(handlerCtx, card)).To(Succeed())
		Expect((&mcpserver.McpServerHandler{}).CreateOrUpdate(handlerCtx, mcp)).To(Succeed())
		Expect(card.Status.Active).To(BeTrue())
		Expect(mcp.Status.Active).To(BeTrue())
	})

	It("does not block registrations in different environments", func() {
		card := &agenticv1.AgentCard{
			ObjectMeta: registrationMetadata(namespace, namespace, "agent", "/Shared/Path"),
			Spec:       agenticv1.AgentCardSpec{BasePath: "/Shared/Path", Name: "Agent", Version: "1.0"},
		}
		mcp := &agenticv1.McpServer{
			ObjectMeta: registrationMetadata(otherNamespace, otherNamespace, "mcp", "/shared/path"),
			Spec:       agenticv1.McpServerSpec{BasePath: "/shared/path", Name: "MCP", Version: "1.0"},
		}
		Expect(k8sClient.Create(ctx, card)).To(Succeed())
		Expect(k8sClient.Create(ctx, mcp)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(card), card)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcp), mcp)).To(Succeed())
		card.TypeMeta = metav1.TypeMeta{APIVersion: agenticv1.GroupVersion.String(), Kind: "AgentCard"}
		mcp.TypeMeta = metav1.TypeMeta{APIVersion: agenticv1.GroupVersion.String(), Kind: "McpServer"}
		cardCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, namespace)))
		mcpCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, otherNamespace)))

		Expect((&agentcard.AgentCardHandler{}).CreateOrUpdate(cardCtx, card)).To(Succeed())
		Expect((&mcpserver.McpServerHandler{}).CreateOrUpdate(mcpCtx, mcp)).To(Succeed())
		Expect(card.Status.Active).To(BeTrue())
		Expect(mcp.Status.Active).To(BeTrue())
	})

	DescribeTable("blocks the newer kind for a normalized path conflict",
		func(cardPath, serverPath string, cardWins bool) {
			card, mcp := createRegistrationPair(namespace, namespace, namespace, "agent", "mcp", cardPath, serverPath)
			cardTime, serverTime := card.CreationTimestamp, mcp.CreationTimestamp
			if cardWins {
				card.CreationTimestamp = metav1.NewTime(serverTime.Add(-time.Hour))
				mcp.CreationTimestamp = metav1.NewTime(cardTime.Add(time.Hour))
			} else {
				card.CreationTimestamp = metav1.NewTime(serverTime.Add(time.Hour))
				mcp.CreationTimestamp = metav1.NewTime(cardTime.Add(-time.Hour))
			}
			handlerCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, namespace)))

			Expect((&agentcard.AgentCardHandler{}).CreateOrUpdate(handlerCtx, card)).To(Succeed())
			Expect((&mcpserver.McpServerHandler{}).CreateOrUpdate(handlerCtx, mcp)).To(Succeed())
			Expect(card.Status.Active).To(Equal(cardWins))
			Expect(mcp.Status.Active).To(Equal(!cardWins))
			if cardWins {
				expectRegistrationBlocked(Default, mcp.GetConditions(), "AgentCard", cardPath)
			} else {
				expectRegistrationBlocked(Default, card.GetConditions(), "McpServer", serverPath)
			}
		},
		Entry("older AgentCard with different case", "/Shared/Path", "/shared/path", true),
		Entry("older McpServer with different case", "/Shared/Path", "/shared/path", false),
		Entry("paths differing beyond case normalize identically", "/shared/path", "/shared-path", true),
	)

	DescribeTable("reconciles both kinds after the winner changes path and the next winner is deleted",
		func(cardFirst, differentCase bool) {
			managerCtx, cancelManager := context.WithCancel(ctx)
			skipNameValidation := true
			mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{
					namespace: {},
				}},
				Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
				Controller: controllerconfig.Controller{SkipNameValidation: &skipNameValidation},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect((&AgentCardReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr)).To(Succeed())
			Expect((&McpServerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr)).To(Succeed())
			done := make(chan error, 1)
			go func() { done <- mgr.Start(managerCtx) }()
			DeferCleanup(func() {
				cancelManager()
				Eventually(done, timeout, interval).Should(Receive(Succeed()))
			})
			syncCtx, cancelSync := context.WithTimeout(managerCtx, timeout)
			defer cancelSync()
			Expect(mgr.GetCache().WaitForCacheSync(syncCtx)).To(BeTrue())

			const oldPath, newPath = "/shared/old", "/shared/new"
			cardPath := oldPath
			if differentCase {
				cardPath = "/Shared/Old"
			}
			cardName, mcpName := "a", "b"
			if !cardFirst {
				cardName, mcpName = "b", "a"
			}
			card := &agenticv1.AgentCard{
				ObjectMeta: registrationMetadata(namespace, namespace, cardName, cardPath),
				Spec:       agenticv1.AgentCardSpec{BasePath: cardPath, Name: "Agent", Version: "1.0"},
			}
			mcp := &agenticv1.McpServer{
				ObjectMeta: registrationMetadata(namespace, namespace, mcpName, oldPath),
				Spec:       agenticv1.McpServerSpec{BasePath: oldPath, Name: "MCP", Version: "1.0"},
			}
			if cardFirst {
				Expect(k8sClient.Create(ctx, card)).To(Succeed())
				Expect(k8sClient.Create(ctx, mcp)).To(Succeed())
			} else {
				Expect(k8sClient.Create(ctx, mcp)).To(Succeed())
				Expect(k8sClient.Create(ctx, card)).To(Succeed())
			}
			thirdCard := &agenticv1.AgentCard{
				ObjectMeta: registrationMetadata(namespace, namespace, "c", oldPath),
				Spec:       agenticv1.AgentCardSpec{BasePath: oldPath, Name: "Third", Version: "1.0"},
			}
			var thirdServer *agenticv1.McpServer
			if cardFirst {
				Expect(k8sClient.Create(ctx, thirdCard)).To(Succeed())
			} else {
				thirdServer = &agenticv1.McpServer{
					ObjectMeta: registrationMetadata(namespace, namespace, "c", oldPath),
					Spec:       agenticv1.McpServerSpec{BasePath: oldPath, Name: "Third", Version: "1.0"},
				}
				Expect(k8sClient.Create(ctx, thirdServer)).To(Succeed())
			}

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(card), card)).To(Succeed())
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcp), mcp)).To(Succeed())
				g.Expect(card.Status.Active).To(Equal(cardFirst))
				g.Expect(mcp.Status.Active).To(Equal(!cardFirst))
				if cardFirst {
					expectRegistrationBlocked(g, mcp.GetConditions(), "AgentCard", cardPath)
				} else {
					expectRegistrationBlocked(g, card.GetConditions(), "McpServer", oldPath)
				}
			}, timeout, interval).Should(Succeed())

			if cardFirst {
				card.Spec.BasePath = newPath
				card.Labels[agenticv1.BasePathLabelKey] = labelutil.NormalizeLabelValue(newPath)
				Expect(k8sClient.Update(ctx, card)).To(Succeed())
			} else {
				mcp.Spec.BasePath = newPath
				mcp.Labels[agenticv1.BasePathLabelKey] = labelutil.NormalizeLabelValue(newPath)
				Expect(k8sClient.Update(ctx, mcp)).To(Succeed())
			}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(card), card)).To(Succeed())
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcp), mcp)).To(Succeed())
				g.Expect(card.Status.Active).To(BeTrue())
				g.Expect(mcp.Status.Active).To(BeTrue())
				if cardFirst {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(thirdCard), thirdCard)).To(Succeed())
					g.Expect(thirdCard.Status.Active).To(BeFalse())
					expectRegistrationBlocked(g, thirdCard.GetConditions(), "McpServer", oldPath)
				} else {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(thirdServer), thirdServer)).To(Succeed())
					g.Expect(thirdServer.Status.Active).To(BeFalse())
					expectRegistrationBlocked(g, thirdServer.GetConditions(), "AgentCard", cardPath)
				}
			}, timeout, interval).Should(Succeed())

			if cardFirst {
				Expect(k8sClient.Delete(ctx, mcp)).To(Succeed())
			} else {
				Expect(k8sClient.Delete(ctx, card)).To(Succeed())
			}
			Eventually(func(g Gomega) {
				if cardFirst {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(thirdCard), thirdCard)).To(Succeed())
					g.Expect(thirdCard.Status.Active).To(BeTrue())
				} else {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(thirdServer), thirdServer)).To(Succeed())
					g.Expect(thirdServer.Status.Active).To(BeTrue())
				}
			}, timeout, interval).Should(Succeed(), fmt.Sprintf("remaining %s registration should take over", map[bool]string{true: "AgentCard", false: "McpServer"}[cardFirst]))
		},
		Entry("AgentCard moves, McpServer is deleted", true, false),
		Entry("McpServer moves, AgentCard is deleted", false, false),
		Entry("case-conflicting AgentCard moves, McpServer is deleted", true, true),
		Entry("case-conflicting McpServer moves, AgentCard is deleted", false, true),
	)
})
