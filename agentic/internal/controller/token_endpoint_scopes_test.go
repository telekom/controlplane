// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/json"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const tokenEndpoint = "https://external.example/token"

func externalIDPFixture() *agenticv1.ExternalIdentityProvider {
	return &agenticv1.ExternalIdentityProvider{
		TokenEndpoint: tokenEndpoint, TokenRequest: agenticv1.TokenRequestClientSecretBasic, GrantType: "client_credentials",
	}
}

type scopeFixture struct {
	namespace          string
	basePath           string
	zone               *adminv1.Zone
	provider, consumer *applicationv1.Application
}

func newScopeFixture() *scopeFixture {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "agentic-scopes-"}}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	f := &scopeFixture{namespace: ns.Name, basePath: "/mcp/" + ns.Name}
	presets := []adminv1.GatewayConfigPreset{{Name: "default", Default: true, Urls: []adminv1.UrlConfig{{Hostname: "gateway.example.com", Scheme: "https", Port: 443, BasePath: "/"}}}}
	f.zone = &adminv1.Zone{
		ObjectMeta: f.metadata("zone"),
		Spec: adminv1.ZoneSpec{
			Visibility:       adminv1.ZoneVisibilityWorld,
			IdentityProvider: adminv1.IdentityProviderConfig{Url: "https://identity.example.com"},
			Gateway:          adminv1.GatewayConfig{Admin: adminv1.GatewayAdminConfig{Url: "https://gateway-admin.example.com"}, Presets: presets},
			AiGateway:        &adminv1.AiGatewayConfig{Admin: adminv1.GatewayAdminConfig{Url: "https://ai-admin.example.com"}, Presets: presets},
		},
	}
	Expect(k8sClient.Create(ctx, f.zone)).To(Succeed())
	f.zone.Status = adminv1.ZoneStatus{
		Namespace: ns.Name,
		AiGateway: &ctypes.ObjectRef{Name: "gateway", Namespace: ns.Name},
		Links:     adminv1.Links{Url: "https://gateway.example.com", Issuer: "https://identity.example.com", LmsIssuer: "https://lms.example.com"},
		Features:  []adminv1.Feature{{Name: adminv1.FeatureAiGateway, Enabled: true}},
	}
	setFixtureReady(f.zone)
	Expect(k8sClient.Status().Update(ctx, f.zone)).To(Succeed())
	f.provider = f.application("provider")
	f.consumer = f.application("consumer")
	return f
}

func (f *scopeFixture) metadata(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: f.namespace, Labels: map[string]string{
		config.EnvironmentLabelKey:        f.namespace,
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(f.basePath),
	}}
}

func setFixtureReady(obj ctypes.Object) {
	c := condition.NewReadyCondition("FixtureReady", "Ready prerequisite supplied by envtest")
	c.ObservedGeneration = obj.GetGeneration()
	obj.SetCondition(c)
}

func (f *scopeFixture) application(name string) *applicationv1.Application {
	app := &applicationv1.Application{ObjectMeta: f.metadata(name), Spec: applicationv1.ApplicationSpec{
		Team: name, TeamEmail: name + "@example.com", Secret: "fixture-secret",
	}}
	Expect(k8sClient.Create(ctx, app)).To(Succeed())
	app.Status.ClientId = name
	setFixtureReady(app)
	Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())
	return app
}

func (f *scopeFixture) server(kind string, scopes []string) {
	if kind == "AgentCard" {
		card := &agenticv1.AgentCard{ObjectMeta: f.metadata("server"), Spec: agenticv1.AgentCardSpec{
			BasePath: f.basePath, Name: "Scope test agent", Version: "1.0.0", Oauth2Scopes: scopes,
		}}
		delete(card.Labels, agenticv1.AgenticBasePathLabelKey)
		card.Labels[agenticv1.AgentBasePathLabelKey] = labelutil.NormalizeLabelValue(f.basePath)
		Expect(k8sClient.Create(ctx, card)).To(Succeed())
		card.Status.Active = true
		setFixtureReady(card)
		Expect(k8sClient.Status().Update(ctx, card)).To(Succeed())
		return
	}
	server := &agenticv1.McpServer{ObjectMeta: f.metadata("server"), Spec: agenticv1.McpServerSpec{
		BasePath: f.basePath, Name: "Scope test MCP", Version: "1.0.0", Oauth2Scopes: scopes,
	}}
	Expect(k8sClient.Create(ctx, server)).To(Succeed())
	server.Status.Active = true
	setFixtureReady(server)
	Expect(k8sClient.Status().Update(ctx, server)).To(Succeed())
}

func (f *scopeFixture) exposure(name string, scopes []string, endpoint bool) *agenticv1.AgenticExposure {
	exp := &agenticv1.AgenticExposure{ObjectMeta: f.metadata(name), Spec: agenticv1.AgenticExposureSpec{
		BasePath: f.basePath, Variant: agenticv1.AgenticVariantMCP,
		Upstreams:  []agenticv1.Upstream{{Url: "https://upstream.example.com/mcp", Weight: 100}},
		Visibility: agenticv1.VisibilityEnterprise,
		Approval:   agenticv1.Approval{Strategy: agenticv1.ApprovalStrategySimple},
		Zone:       *ctypes.ObjectRefFromObject(f.zone), Provider: *ctypes.ObjectRefFromObject(f.provider),
		Security: &agenticv1.Security{M2M: &agenticv1.Machine2MachineAuthentication{Scopes: scopes}},
	}}
	if endpoint {
		exp.Spec.Security.M2M.ExternalIDP = externalIDPFixture()
	}
	Expect(k8sClient.Create(ctx, exp)).To(Succeed())
	return exp
}

func (f *scopeFixture) subscription(scopes []string) *agenticv1.AgenticSubscription {
	sub := &agenticv1.AgenticSubscription{ObjectMeta: f.metadata("subscription"), Spec: agenticv1.AgenticSubscriptionSpec{
		BasePath: f.basePath, Zone: *ctypes.ObjectRefFromObject(f.zone),
		Requestor: agenticv1.Requestor{Application: *ctypes.ObjectRefFromObject(f.consumer)},
		Security:  &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{Scopes: scopes}},
	}}
	Expect(k8sClient.Create(ctx, sub)).To(Succeed())
	return sub
}

func expectReadyReason(g Gomega, obj ctypes.Object, reason string, status metav1.ConditionStatus) {
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	c := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	g.Expect(c).NotTo(BeNil())
	g.Expect(c.Reason).To(Equal(reason))
	g.Expect(c.Status).To(Equal(status))
	g.Expect(c.ObservedGeneration).To(Equal(obj.GetGeneration()))
}

func waitExposure(exp *agenticv1.AgenticExposure) *gatewayv1.Route {
	route := &gatewayv1.Route{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exp), exp)).To(Succeed())
		g.Expect(exp.Status.Route).NotTo(BeNil())
		g.Expect(k8sClient.Get(ctx, exp.Status.Route.K8s(), route)).To(Succeed())
	}, timeout, interval).Should(Succeed())
	updateFixtureStatus(route)
	Eventually(func(g Gomega) {
		expectReadyReason(g, exp, "AgenticExposureProvisioned", metav1.ConditionTrue)
		g.Expect(exp.Status.Active).To(BeTrue())
	}, timeout, interval).Should(Succeed())
	return route
}

func updateFixtureStatus(obj ctypes.Object) {
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		if condition.IsReady(obj) {
			return
		}
		setFixtureReady(obj)
		g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
	}, timeout, interval).Should(Succeed())
}

func waitPending(sub *agenticv1.AgenticSubscription) *approvalv1.ApprovalRequest {
	req := &approvalv1.ApprovalRequest{}
	Eventually(func(g Gomega) {
		expectReadyReason(g, sub, "ApprovalPending", metav1.ConditionFalse)
		g.Expect(sub.Status.ApprovalRequest).NotTo(BeNil())
		g.Expect(sub.Status.Approval).NotTo(BeNil())
		g.Expect(k8sClient.Get(ctx, sub.Status.ApprovalRequest.K8s(), req)).To(Succeed())
		g.Expect(req.Spec.State).To(Equal(approvalv1.ApprovalStatePending))
		g.Expect(req.Spec.Strategy).To(Equal(approvalv1.ApprovalStrategySimple))
	}, timeout, interval).Should(Succeed())
	return req
}

// Approval controllers are absent; supply linked decisions through the real API only after pending is observed.
func grantApproval(sub *agenticv1.AgenticSubscription, req *approvalv1.ApprovalRequest) *approvalv1.Approval {
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(req), req)).To(Succeed())
		req.Spec.State = approvalv1.ApprovalStateGranted
		g.Expect(k8sClient.Update(ctx, req)).To(Succeed())
	}, timeout, interval).Should(Succeed())
	updateFixtureStatus(req)
	approval := &approvalv1.Approval{ObjectMeta: metav1.ObjectMeta{
		Name: sub.Status.Approval.Name, Namespace: sub.Namespace,
		Labels: map[string]string{config.EnvironmentLabelKey: sub.Labels[config.EnvironmentLabelKey]},
	}, Spec: approvalv1.ApprovalSpec{
		State: approvalv1.ApprovalStateGranted, Target: req.Spec.Target, Action: req.Spec.Action,
		Requester: req.Spec.Requester, Decider: req.Spec.Decider, Strategy: req.Spec.Strategy,
		ApprovedRequest: ctypes.ObjectRefFromObject(req),
	}}
	Expect(controllerutil.SetControllerReference(sub, approval, k8sClient.Scheme())).To(Succeed())
	Expect(k8sClient.Create(ctx, approval)).To(Succeed())
	approval.Status.LastState = approvalv1.ApprovalStateGranted
	setFixtureReady(approval)
	Expect(k8sClient.Status().Update(ctx, approval)).To(Succeed())
	return approval
}

func waitConsumeRoute(sub *agenticv1.AgenticSubscription, scopes []string) *gatewayv1.ConsumeRoute {
	consume := &gatewayv1.ConsumeRoute{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
		g.Expect(sub.Status.ConsumeRoute).NotTo(BeNil())
		g.Expect(k8sClient.Get(ctx, sub.Status.ConsumeRoute.K8s(), consume)).To(Succeed())
		g.Expect(consume.Spec.Security.M2M).NotTo(BeNil())
		g.Expect(consume.Spec.Security.M2M.Scopes).To(Equal(scopes))
		// Subscription changes can also update the exposure Route's allowed consumers.
		route := &gatewayv1.Route{}
		g.Expect(k8sClient.Get(ctx, consume.Spec.Route.K8s(), route)).To(Succeed())
		if !condition.IsReady(route) {
			setFixtureReady(route)
			g.Expect(k8sClient.Status().Update(ctx, route)).To(Succeed())
		}
		if !condition.IsReady(consume) {
			setFixtureReady(consume)
			g.Expect(k8sClient.Status().Update(ctx, consume)).To(Succeed())
		}
		expectReadyReason(g, sub, condition.ReasonProvisioned, metav1.ConditionTrue)
	}, timeout, interval).Should(Succeed())
	return consume
}

func (f *scopeFixture) expectNoProvisioning(g Gomega) {
	consume := &gatewayv1.ConsumeRouteList{}
	g.Expect(k8sClient.List(ctx, consume, client.InNamespace(f.namespace))).To(Succeed())
	g.Expect(consume.Items).To(BeEmpty())
	requests := &approvalv1.ApprovalRequestList{}
	g.Expect(k8sClient.List(ctx, requests, client.InNamespace(f.namespace))).To(Succeed())
	g.Expect(requests.Items).To(BeEmpty())
}

// A quiet resource-version window prevents approval/child events from becoming a recovery trigger.
func settleScopeObjects(objects ...client.Object) {
	var previous []string
	unchangedSince := time.Now()
	Eventually(func(g Gomega) {
		versions := make([]string, len(objects))
		for i, obj := range objects {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
			versions[i] = obj.GetResourceVersion()
		}
		if !reflect.DeepEqual(previous, versions) {
			previous = versions
			unchangedSince = time.Now()
		}
		g.Expect(time.Since(unchangedSince)).To(BeNumerically(">=", time.Second))
	}, timeout, interval).Should(Succeed())
	Consistently(func(g Gomega) {
		for i, obj := range objects {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
			g.Expect(obj.GetResourceVersion()).To(Equal(previous[i]))
		}
	}, time.Second, interval).Should(Succeed())
}

var _ = Describe("External token endpoint scope exception", func() {
	DescribeTable("looks up the real backing server", func(kind string) {
		f := newScopeFixture()
		declared := []string{"write", "read"}
		f.server(kind, declared)
		lookupCtx := cclient.WithClient(ctx, cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, f.namespace)))
		server, conflict, err := util.ServerMustExist(lookupCtx, f.basePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(conflict).To(BeFalse())
		Expect(server).To(Equal(&util.ServerInfo{BasePath: f.basePath, Oauth2Scopes: declared}))
	}, Entry("McpServer", "McpServer"), Entry("AgentCard with its own base-path label", "AgentCard"))

	It("forwards unmatched provider and consumer scopes and updates both persisted resources in place", func() {
		f := newScopeFixture()
		f.server("McpServer", []string{"spec.read"})
		providerScopes, consumerScopes := []string{"provider.z", "provider.a", "provider.z"}, []string{"consumer.z", "consumer.a", "consumer.z"}
		exp := f.exposure("exposure", providerScopes, true)
		route := waitExposure(exp)
		Expect(route.Spec.Security.M2M.Scopes).To(Equal(providerScopes))
		sub := f.subscription(consumerScopes)
		req := waitPending(sub)
		grantApproval(sub, req)
		consume := waitConsumeRoute(sub, consumerScopes)
		Expect(route.OwnerReferences).To(BeEmpty())
		Expect(consume.OwnerReferences).To(HaveLen(1))
		Expect(consume.OwnerReferences[0].UID).To(Equal(sub.UID))
		routeBefore, consumeBefore := route.DeepCopy(), consume.DeepCopy()
		requestUID := req.UID

		providerScopes, consumerScopes = []string{"provider.new.b", "provider.new.a"}, []string{"consumer.new.b", "consumer.new.a"}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exp), exp)).To(Succeed())
			exp.Spec.Security.M2M.Scopes = providerScopes
			g.Expect(k8sClient.Update(ctx, exp)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			sub.Spec.Security.M2M.Scopes = consumerScopes
			g.Expect(k8sClient.Update(ctx, sub)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			g.Expect(route.Spec.Security.M2M.Scopes).To(Equal(providerScopes))
		}, timeout, interval).Should(Succeed())
		waitExposure(exp)
		consume = waitConsumeRoute(sub, consumerScopes)
		Eventually(func(g Gomega) {
			expectReadyReason(g, exp, "AgenticExposureProvisioned", metav1.ConditionTrue)
			expectReadyReason(g, sub, condition.ReasonProvisioned, metav1.ConditionTrue)
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			g.Expect(route.Spec.Security.M2M.Scopes).To(Equal(providerScopes))
			g.Expect(consume.Spec.Security.M2M.Scopes).To(Equal(consumerScopes))
			g.Expect(route.UID).To(Equal(routeBefore.UID))
			g.Expect(consume.UID).To(Equal(consumeBefore.UID))
			g.Expect(route.OwnerReferences).To(Equal(routeBefore.OwnerReferences))
			g.Expect(consume.OwnerReferences).To(Equal(consumeBefore.OwnerReferences))
			g.Expect(k8sClient.Get(ctx, sub.Status.ApprovalRequest.K8s(), req)).To(Succeed())
			g.Expect(req.UID).To(Equal(requestUID))
		}, timeout, interval).Should(Succeed())
		var properties map[string]any
		Expect(json.Unmarshal(req.Spec.Requester.Properties.Raw, &properties)).To(Succeed())
		Expect(properties).To(Equal(map[string]any{"mcpBasePath": f.basePath, "resource_type": "MCP", "resource_name": f.basePath}))
	})

	It("does not borrow an inactive competing exposure's endpoint", func() {
		f := newScopeFixture()
		f.server("McpServer", []string{"read"})
		active := f.exposure("active", []string{"read"}, false)
		waitExposure(active)
		inactive := f.exposure("inactive", []string{"provider"}, true)
		Eventually(func(g Gomega) {
			expectReadyReason(g, inactive, "AgenticExposureAlreadyExists", metav1.ConditionFalse)
			g.Expect(inactive.Status.Active).To(BeFalse())
		}, timeout, interval).Should(Succeed())
		sub := f.subscription([]string{"consumer"})
		Eventually(func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			f.expectNoProvisioning(g)
		}, timeout, interval).Should(Succeed())
	})

	DescribeTable("preserves missing-exposure diagnostics", func(scopes []string, reason, message string) {
		f := newScopeFixture()
		f.server("McpServer", []string{"read"})
		sub := f.subscription(scopes)
		Eventually(func(g Gomega) {
			expectReadyReason(g, sub, reason, metav1.ConditionFalse)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(ContainSubstring(message))
			f.expectNoProvisioning(g)
		}, timeout, interval).Should(Succeed())
	}, Entry("scope failure precedes missing exposure", []string{"consumer"}, condition.ReasonValidationFailed, "not defined in the server"),
		Entry("valid scopes still require an exposure", []string{"read"}, condition.ReasonPreconditionNotMet, "No active AgenticExposure found"))

	DescribeTable("preserves the selected exposure's readiness gate", func(endpoint bool, scopes []string, reason, message string) {
		f := newScopeFixture()
		f.server("McpServer", []string{"read"})
		exp := f.exposure("exposure", []string{"read"}, endpoint)
		route := waitExposure(exp)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			notReady := condition.NewNotReadyCondition("FixtureBlocked", "Gateway is not ready")
			notReady.ObservedGeneration = route.Generation
			route.SetCondition(notReady)
			g.Expect(k8sClient.Status().Update(ctx, route)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			expectReadyReason(g, exp, "ChildResourcesNotReady", metav1.ConditionFalse)
			g.Expect(exp.Status.Active).To(BeTrue())
		}, timeout, interval).Should(Succeed())
		sub := f.subscription(scopes)
		Eventually(func(g Gomega) {
			expectReadyReason(g, sub, reason, metav1.ConditionFalse)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(ContainSubstring(message))
			f.expectNoProvisioning(g)
		}, timeout, interval).Should(Succeed())
	}, Entry("non-exempt scope failure precedes readiness", false, []string{"consumer"}, condition.ReasonValidationFailed, "not defined in the server"),
		Entry("valid non-exempt scopes still require readiness", false, []string{"read"}, condition.ReasonPreconditionNotMet, "is not ready"),
		Entry("qualifying endpoint does not exempt readiness", true, []string{"consumer"}, condition.ReasonPreconditionNotMet, "is not ready"))

	It("revalidates through exposure-only endpoint add/remove watches before any approval events", func() {
		f := newScopeFixture()
		f.server("McpServer", []string{"read"})
		exp := f.exposure("exposure", []string{"read"}, false)
		route := waitExposure(exp)
		consumerScopes := []string{"external.write", "external.read"}
		sub := f.subscription(consumerScopes)
		Eventually(func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			f.expectNoProvisioning(g)
		}, timeout, interval).Should(Succeed())
		settleScopeObjects(exp, route, sub)
		Consistently(func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			f.expectNoProvisioning(g)
		}, time.Second, interval).Should(Succeed())
		subGeneration := sub.Generation
		exp.Spec.Security.M2M.ExternalIDP = externalIDPFixture()
		Expect(k8sClient.Update(ctx, exp)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			g.Expect(route.Spec.Security.M2M.ExternalIDP).NotTo(BeNil())
		}, timeout, interval).Should(Succeed())
		waitExposure(exp)
		req := waitPending(sub)
		Expect(sub.Generation).To(Equal(subGeneration))
		Expect(req.CreationTimestamp.Before(&exp.CreationTimestamp)).To(BeFalse())
		approval := grantApproval(sub, req)
		consume := waitConsumeRoute(sub, consumerScopes)
		Eventually(func(g Gomega) {
			expectReadyReason(g, exp, "AgenticExposureProvisioned", metav1.ConditionTrue)
		}, timeout, interval).Should(Succeed())
		settleScopeObjects(exp, route, sub, req, approval, consume)

		exp.Spec.Security.M2M.ExternalIDP = nil
		Expect(k8sClient.Update(ctx, exp)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			g.Expect(route.Spec.Security.M2M.ExternalIDP).To(BeNil())
		}, timeout, interval).Should(Succeed())
		waitExposure(exp)
		Eventually(func(g Gomega) {
			expectReadyReason(g, exp, "AgenticExposureProvisioned", metav1.ConditionTrue)
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			g.Expect(sub.Generation).To(Equal(subGeneration))
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(consume), consume)).To(Succeed())
			g.Expect(consume.Spec.Security.M2M.Scopes).To(Equal(consumerScopes))
		}, timeout, interval).Should(Succeed())
	})
})
