// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	organizationapi "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const scopeTimeout = 30 * time.Second

// newScopeFixtures supplies isolated, real prerequisites for the running controllers.
// The caller creates the exposure and subscription after configuring their scopes.
func newScopeFixtures(name string, declared []string, providerReady bool) (*apiapi.ApiExposure, *apiapi.ApiSubscription) {
	namespace := testEnvironment + "--" + testGroup + "--" + name
	CreateNamespace(namespace)
	zone := CreateZone(name)
	team := &organizationapi.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name: testGroup + "--" + name, Namespace: testEnvironment,
			Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
		},
		Spec: organizationapi.TeamSpec{
			Name: name, Group: testGroup, Email: "scopes@example.com", Category: testCategory,
			Members: []organizationapi.Member{{Name: "Scope Tester", Email: "scopes@example.com"}},
		},
	}
	Expect(k8sClient.Create(ctx, team)).To(Succeed())
	for _, role := range []string{"provider", "consumer"} {
		app := &applicationapi.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name: role, Namespace: namespace,
				Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: applicationapi.ApplicationSpec{Team: name, TeamEmail: "scopes@example.com", Secret: "test-secret"},
		}
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		app.Status.ClientId = name + "-" + role
		app.Status.ClientSecret = "test-secret"
		ready := condition.NewReadyCondition("Ready", "Test prerequisite")
		if role == "provider" && !providerReady {
			ready = condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, "Provider is not ready")
		}
		ready.ObservedGeneration = app.Generation
		app.SetCondition(ready)
		Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())
	}
	api := NewApi("/" + name + "/v1")
	api.Namespace = namespace
	api.Spec.Oauth2Scopes = declared
	Expect(k8sClient.Create(ctx, api)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(api), api)).To(Succeed())
		g.Expect(api.Status.Active).To(BeTrue())
		expectScopeCondition(g, api.GetConditions(), condition.ConditionTypeReady, metav1.ConditionTrue, condition.ReasonProvisioned, api.Generation)
	}, scopeTimeout, interval).Should(Succeed())

	exposure := NewApiExposure(api.Spec.BasePath, name, "provider")
	exposure.Namespace = namespace
	exposure.Spec.Approval.Strategy = apiapi.ApprovalStrategySimple
	sub := NewApiSubscription(api.Spec.BasePath, name, "consumer")
	sub.Namespace = namespace
	sub.Spec.Requestor.Application.Namespace = namespace

	DeferCleanup(func() {
		for _, objects := range []client.ObjectList{&apiapi.ApiSubscriptionList{}, &apiapi.ApiExposureList{}, &apiapi.ApiList{}} {
			var object client.Object
			switch objects.(type) {
			case *apiapi.ApiSubscriptionList:
				object = &apiapi.ApiSubscription{}
			case *apiapi.ApiExposureList:
				object = &apiapi.ApiExposure{}
			case *apiapi.ApiList:
				object = &apiapi.Api{}
			}
			Expect(k8sClient.DeleteAllOf(ctx, object, client.InNamespace(namespace))).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.List(ctx, objects, client.InNamespace(namespace))).To(Succeed())
				items, err := meta.ExtractList(objects)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(items).To(BeEmpty())
			}, scopeTimeout, interval).Should(Succeed())
		}
		Expect(k8sClient.Delete(ctx, team)).To(Succeed())
		Expect(k8sClient.Delete(ctx, zone)).To(Succeed())
		for _, ns := range []string{namespace, zone.Status.Namespace} {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		}
	})
	return exposure, sub
}

func expectScopeCondition(g Gomega, conditions []metav1.Condition, kind string, status metav1.ConditionStatus, reason string, generation int64) {
	c := meta.FindStatusCondition(conditions, kind)
	g.Expect(c).NotTo(BeNil())
	g.Expect(c.Status).To(Equal(status))
	g.Expect(c.Reason).To(Equal(reason))
	g.Expect(c.ObservedGeneration).To(Equal(generation))
}

func expectScopeExposureReady(g Gomega, exposure *apiapi.ApiExposure) {
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exposure), exposure)).To(Succeed())
	g.Expect(exposure.Status.Active).To(BeTrue())
	expectScopeCondition(g, exposure.GetConditions(), condition.ConditionTypeReady, metav1.ConditionTrue, condition.ReasonProvisioned, exposure.Generation)
}

func expectScopePending(g Gomega, sub *apiapi.ApiSubscription) {
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
	expectScopeCondition(g, sub.GetConditions(), "ScopesAllowed", metav1.ConditionTrue, "Allowed", sub.Generation)
	expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionFalse, condition.ReasonApprovalPending, sub.Generation)
	g.Expect(sub.Status.ConsumeRoute).To(BeNil())
	g.Expect(sub.Status.ApprovalRequest).NotTo(BeNil())
	request := &approvalapi.ApprovalRequest{}
	g.Expect(k8sClient.Get(ctx, sub.Status.ApprovalRequest.K8s(), request)).To(Succeed())
	g.Expect(request.Spec.Strategy).To(Equal(approvalapi.ApprovalStrategySimple))
	var properties struct {
		Scopes       []string `json:"scopes"`
		BasePath     string   `json:"basePath"`
		ResourceType string   `json:"resource_type"`
		ResourceName string   `json:"resource_name"`
	}
	g.Expect(json.Unmarshal(request.Spec.Requester.Properties.Raw, &properties)).To(Succeed())
	g.Expect(properties.Scopes).To(Equal(sub.Spec.Security.M2M.Scopes))
	g.Expect(properties.BasePath).To(Equal(sub.Spec.ApiBasePath))
	g.Expect(properties.ResourceName).To(Equal(sub.Spec.ApiBasePath))
	g.Expect(properties.ResourceType).To(Equal("API"))
}

func expectScopeRoutes(g Gomega, exposure *apiapi.ApiExposure, sub *apiapi.ApiSubscription) {
	expectScopeExposureReady(g, exposure)
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
	expectScopeCondition(g, sub.GetConditions(), "ScopesAllowed", metav1.ConditionTrue, "Allowed", sub.Generation)
	expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionTrue, condition.ReasonProvisioned, sub.Generation)
	g.Expect(exposure.Status.Route).NotTo(BeNil())
	route := &gatewayapi.Route{}
	g.Expect(k8sClient.Get(ctx, exposure.Status.Route.K8s(), route)).To(Succeed())
	g.Expect(route.Spec.Security).NotTo(BeNil())
	g.Expect(route.Spec.Security.M2M).NotTo(BeNil())
	g.Expect(route.Spec.Security.M2M.Scopes).To(Equal(exposure.Spec.Security.M2M.Scopes))
	g.Expect(sub.Status.ConsumeRoute).NotTo(BeNil())
	consume := &gatewayapi.ConsumeRoute{}
	g.Expect(k8sClient.Get(ctx, sub.Status.ConsumeRoute.K8s(), consume)).To(Succeed())
	g.Expect(consume.Spec.Security).NotTo(BeNil())
	g.Expect(consume.Spec.Security.M2M).NotTo(BeNil())
	g.Expect(consume.Spec.Security.M2M.Scopes).To(Equal(sub.Spec.Security.M2M.Scopes))
}

func expectScopeBlocked(g Gomega, sub *apiapi.ApiSubscription) {
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
	expectScopeCondition(g, sub.GetConditions(), "ScopesAllowed", metav1.ConditionFalse, "NotAllowed", sub.Generation)
	expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionFalse, condition.ReasonValidationFailed, sub.Generation)
	g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(Equal("One or more scopes which are defined in ApiSubscription are not defined in the ApiSpecification"))
}

// Stable resource versions rule out continuing approval/child/status activity before an endpoint edit.
func settleScopeResources(objects ...client.Object) {
	versions := make([]string, len(objects))
	Eventually(func(g Gomega) {
		for i, object := range objects {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(object), object)).To(Succeed())
			version := object.GetResourceVersion()
			previous := versions[i]
			versions[i] = version
			g.Expect(version).To(Equal(previous))
		}
	}, scopeTimeout, interval).MustPassRepeatedly(10).Should(Succeed())
	Consistently(func(g Gomega) {
		for i, object := range objects {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(object), object)).To(Succeed())
			g.Expect(object.GetResourceVersion()).To(Equal(versions[i]))
		}
	}, time.Second, interval).Should(Succeed())
}

var _ = Describe("External token endpoint scopes", func() {
	DescribeTable("forwards independent provider and consumer scopes through manual approval", func(name string, declared []string) {
		exposure, sub := newScopeFixtures(name, declared, true)
		exposure.Spec.Security.M2M.Scopes = []string{"provider:z", "provider:a", "provider:z"}
		sub.Spec.Security.M2M.Scopes = []string{"consumer:z", "consumer:a", "consumer:z"}
		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeExposureReady(g, exposure) }, scopeTimeout, interval).Should(Succeed())
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		Eventually(func(g Gomega) { expectScopePending(g, sub) }, scopeTimeout, interval).Should(Succeed())
		request := ProgressApprovalRequest(sub.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		ProgressApproval(sub, approvalapi.ApprovalStateGranted, request)
		CompleteSubscriptionChildren(sub)
		Eventually(func(g Gomega) { expectScopeRoutes(g, exposure, sub) }, scopeTimeout, interval).Should(Succeed())
	},
		Entry("without declared scopes", "scope-no-declared", nil),
		Entry("with unmatched declared scopes", "scope-unmatched", []string{"spec:only"}),
	)

	It("does not inherit an inactive competing exposure's endpoint", func() {
		exposure, sub := newScopeFixtures("scope-selection", []string{"scope1"}, true)
		exposure.Spec.Security.M2M.ExternalIDP = nil
		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeExposureReady(g, exposure) }, scopeTimeout, interval).Should(Succeed())
		competing := NewApiExposure(exposure.Spec.ApiBasePath, exposure.Spec.Zone.Name, "provider")
		competing.Namespace = exposure.Namespace
		competing.Name += "-competing"
		Expect(k8sClient.Create(ctx, competing)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(competing), competing)).To(Succeed())
			g.Expect(competing.Status.Active).To(BeFalse())
			expectScopeCondition(g, competing.GetConditions(), "ApiExposureActive", metav1.ConditionFalse, "NotActive", competing.Generation)
		}, scopeTimeout, interval).Should(Succeed())
		sub.Spec.Security.M2M.Scopes = []string{"consumer:external"}
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeBlocked(g, sub) }, scopeTimeout, interval).Should(Succeed())
		Consistently(func(g Gomega) {
			expectScopeBlocked(g, sub)
			g.Expect(sub.Status.ApprovalRequest).To(BeNil())
			g.Expect(sub.Status.ConsumeRoute).To(BeNil())
			consumeRoutes := &gatewayapi.ConsumeRouteList{}
			g.Expect(k8sClient.List(ctx, consumeRoutes, client.InNamespace(testEnvironment+"--"+exposure.Spec.Zone.Name))).To(Succeed())
			g.Expect(consumeRoutes.Items).To(BeEmpty())
		}, time.Second, interval).Should(Succeed())
	})

	It("blocks provisioning when the selected exempt exposure is unready", func() {
		exposure, sub := newScopeFixtures("scope-unready", nil, false)
		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exposure), exposure)).To(Succeed())
			g.Expect(exposure.Status.Active).To(BeTrue())
			g.Expect(exposure.HasExternalIdp()).To(BeTrue())
			expectScopeCondition(g, exposure.GetConditions(), condition.ConditionTypeReady, metav1.ConditionFalse, condition.ReasonPreconditionNotMet, exposure.Generation)
		}, scopeTimeout, interval).Should(Succeed())
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		check := func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			expectScopeCondition(g, sub.GetConditions(), "ApiExposureExists", metav1.ConditionTrue, "ApiExposureExists", sub.Generation)
			expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionFalse, condition.ReasonPreconditionNotMet, sub.Generation)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(Equal(fmt.Sprintf("ApiExposure %q is not ready", exposure.Name)))
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), "ScopesAllowed")).To(BeNil())
			g.Expect(sub.Status.ApprovalRequest).To(BeNil())
			g.Expect(sub.Status.ConsumeRoute).To(BeNil())
			consumeRoutes := &gatewayapi.ConsumeRouteList{}
			g.Expect(k8sClient.List(ctx, consumeRoutes, client.InNamespace(testEnvironment+"--"+exposure.Spec.Zone.Name))).To(Succeed())
			g.Expect(consumeRoutes.Items).To(BeEmpty())
		}
		Eventually(check, scopeTimeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())
	})

	It("revalidates through exposure-only endpoint add/remove watches", func() {
		Expect(config.RequeueAfter).To(Equal(30 * time.Minute))
		exposure, sub := newScopeFixtures("scope-watch", []string{"scope1"}, true)
		idp := exposure.Spec.Security.M2M.ExternalIDP.DeepCopy()
		exposure.Spec.Security.M2M.ExternalIDP = nil
		sub.Spec.Security.M2M.Scopes = []string{"consumer:z", "consumer:a", "consumer:z"}
		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeExposureReady(g, exposure) }, scopeTimeout, interval).Should(Succeed())
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeBlocked(g, sub) }, scopeTimeout, interval).Should(Succeed())
		settleScopeResources(exposure, sub)
		Consistently(func(g Gomega) {
			expectScopeBlocked(g, sub)
			g.Expect(sub.Status.ApprovalRequest).To(BeNil())
		}, time.Second, interval).Should(Succeed())
		generation := sub.Generation

		By("adding only the exposure endpoint and waiting for scope success before granting approval")
		exposure.Spec.Security.M2M.ExternalIDP = idp
		Expect(k8sClient.Update(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) {
			expectScopeExposureReady(g, exposure)
			expectScopePending(g, sub)
			g.Expect(sub.Generation).To(Equal(generation))
		}, scopeTimeout, interval).Should(Succeed())
		request := ProgressApprovalRequest(sub.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		approval := ProgressApproval(sub, approvalapi.ApprovalStateGranted, request)
		CompleteSubscriptionChildren(sub)
		Eventually(func(g Gomega) { expectScopeRoutes(g, exposure, sub) }, scopeTimeout, interval).Should(Succeed())
		route := &gatewayapi.Route{}
		Expect(k8sClient.Get(ctx, exposure.Status.Route.K8s(), route)).To(Succeed())
		consume := &gatewayapi.ConsumeRoute{}
		Expect(k8sClient.Get(ctx, sub.Status.ConsumeRoute.K8s(), consume)).To(Succeed())
		settleScopeResources(exposure, sub, request, approval, route, consume)
		consumeUID := consume.UID

		By("removing only the exposure IDP configuration after approval and child activity settles")
		exposure.Spec.Security.M2M.ExternalIDP = nil
		Expect(k8sClient.Update(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) {
			expectScopeExposureReady(g, exposure)
			expectScopeBlocked(g, sub)
			g.Expect(sub.Generation).To(Equal(generation))
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(consume), consume)).To(Succeed())
			g.Expect(consume.UID).To(Equal(consumeUID))
			g.Expect(consume.Spec.Security.M2M.Scopes).To(Equal(sub.Spec.Security.M2M.Scopes))
		}, scopeTimeout, interval).Should(Succeed())
	})
})
