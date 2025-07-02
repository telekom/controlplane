// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func CreateApplication(name string) *applicationapi.Application {
	app := &applicationapi.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: applicationapi.ApplicationSpec{
			Team:      "Hyperion",
			TeamEmail: "hyperion@test.de",
		},
	}

	err := k8sClient.Create(ctx, app)
	Expect(err).ToNot(HaveOccurred())

	app.Status.ClientId = "test-client-id"
	app.Status.ClientSecret = "topsecret"
	app.SetCondition(condition.NewReadyCondition("AppReady", "Trust me, I am ready"))

	err = k8sClient.Status().Update(ctx, app)
	Expect(err).ToNot(HaveOccurred())

	return app
}

func NewApiSubscription(apiBasePath, zoneName, appName string) *apiapi.ApiSubscription {
	return &apiapi.ApiSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(fmt.Sprintf("%s--%s--%s", zoneName, appName, apiBasePath)),
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiapi.BasePathLabelKey:    labelutil.NormalizeValue(apiBasePath),
			},
		},
		Spec: apiapi.ApiSubscriptionSpec{
			ApiBasePath:  apiBasePath,
			Organization: "",
			Security: apiapi.SubscriberSecurity{
				M2M: &apiapi.SubscriberMachine2MachineAuthentication{
					Scopes: []string{"scope1", "scope2"},
				},
			},
			Requestor: apiapi.Requestor{
				Application: types.ObjectRef{
					Name:      appName,
					Namespace: testNamespace,
				},
			},
			Zone: types.ObjectRef{
				Name:      zoneName,
				Namespace: testEnvironment,
			},
		},
	}
}

func CreateGatewayClient(zone *adminapi.Zone) *identityapi.Client {
	gwClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: zone.Status.Namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: identityapi.ClientSpec{
			Realm: &types.ObjectRef{
				Name:      "test",
				Namespace: zone.Status.Namespace,
			},
			ClientId:     "gateway",
			ClientSecret: "topsecret",
		},
	}

	err := k8sClient.Create(ctx, gwClient)
	Expect(err).ToNot(HaveOccurred())

	gwClient.Status = identityapi.ClientStatus{
		IssuerUrl: "http://localhost:8080/auth/realms/test",
	}
	err = k8sClient.Status().Update(ctx, gwClient)
	Expect(err).ToNot(HaveOccurred())

	return gwClient
}

func ProgressApprovalRequest(ref *types.ObjectRef, state approvalapi.ApprovalState) *approvalapi.ApprovalRequest {
	approvalReq := &approvalapi.ApprovalRequest{}
	err := k8sClient.Get(ctx, ref.K8s(), approvalReq)
	Expect(err).ToNot(HaveOccurred())

	approvalReq.Spec.State = state
	approvalReq.Spec.Decider = approvalapi.Decider{
		Name:    "test-decider",
		Email:   "decider@test.com",
		Comment: "Test-Comment",
	}

	err = k8sClient.Update(ctx, approvalReq)
	Expect(err).ToNot(HaveOccurred())

	return approvalReq
}

func ProgressApproval(apiSub *apiapi.ApiSubscription, state approvalapi.ApprovalState, approvalReq *approvalapi.ApprovalRequest) *approvalapi.Approval {
	approval := &approvalapi.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiSub.Status.Approval.Name,
			Namespace: apiSub.Status.Approval.Namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, approval, func() error {

		err := controllerutil.SetControllerReference(apiSub, approval, k8sClient.Scheme())
		Expect(err).ToNot(HaveOccurred())

		approval.Spec = approvalapi.ApprovalSpec{
			State:           state,
			Resource:        approvalReq.Spec.Resource,
			Action:          approvalReq.Spec.Action,
			Requester:       approvalReq.Spec.Requester,
			Decider:         approvalReq.Spec.Decider,
			Strategy:        approvalReq.Spec.Strategy,
			ApprovedRequest: types.ObjectRefFromObject(approvalReq),
		}
		return nil
	})

	Expect(err).ToNot(HaveOccurred())
	return approval
}

var _ = Describe("ApiSubscription Controller", Ordered, func() {
	// API that is used for the tests
	var apiBasePath = "/apisubctrl/test/v1"

	// Provider side
	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure

	// Provider/Exposure zone
	var zoneName = "apisub-test"
	var zone *adminapi.Zone

	// Consumer/Subscription zone
	var otherZoneName = "other-zone"
	var otherZone *adminapi.Zone

	// Consumer side
	var appName = "my-test-app"
	var application *applicationapi.Application
	var apiSubscription *apiapi.ApiSubscription

	BeforeAll(func() {
		By("Initializing the API, APIExposure and ApiSubscription")
		api = NewApi(apiBasePath)
		apiExposure = NewApiExposure(apiBasePath, zoneName)
		apiSubscription = NewApiSubscription(apiBasePath, zoneName, appName)

		By("Creating the Zone")
		zone = CreateZone(zoneName)
		CreateGatewayClient(zone)

		By("Creating the Realm")
		realm := NewRealm(testEnvironment, zone.Name)
		err := k8sClient.Create(ctx, realm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the Application")
		application = CreateApplication(appName)
	})

	AfterAll(func() {
		By("Deleting the Application")
		err := k8sClient.Delete(ctx, application)
		Expect(err).ToNot(HaveOccurred())

		By("Cleaning up and deleting all resources")
		err = k8sClient.Delete(ctx, api)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Creating and Updating", func() {

		It("should block until an API is exposed", func() {
			By("Creating the resource")
			err := k8sClient.Create(ctx, apiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				processingCondition := meta.FindStatusCondition(apiSubscription.Status.Conditions, condition.ConditionTypeProcessing)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Reason).To(Equal("Blocked"))

			}, timeout, interval).Should(Succeed())

		})

		It("should automatically progress when an API is exposed", func() {
			By("Creating the API resource")
			err := k8sClient.Create(ctx, api)
			Expect(err).ToNot(HaveOccurred())
			By("Creating the APIExposure resource")
			err = k8sClient.Create(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				By("Checking the conditions")
				processingCondition := meta.FindStatusCondition(apiSubscription.Status.Conditions, condition.ConditionTypeProcessing)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Reason).To(Equal("Blocked"))

			}, timeout, interval).Should(Succeed())

		})

		It("should correctly use the approval-workflow", func() {
			By("Checking if the Approval-Request has been created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.ApprovalRequest).ToNot(BeNil())
				approvalRequestRef := apiSubscription.Status.ApprovalRequest
				approvalRequest := &approvalapi.ApprovalRequest{}

				err = k8sClient.Get(ctx, approvalRequestRef.K8s(), approvalRequest)
				g.Expect(err).ToNot(HaveOccurred())

				By("Checking the conditions")
				readyCondition := meta.FindStatusCondition(apiSubscription.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("ApprovalPending"))
				var propertiesMap map[string]interface{}
				err = json.Unmarshal(approvalRequest.Spec.Requester.Properties.Raw, &propertiesMap)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(propertiesMap["scopes"]).To(HaveLen(2))

			}, timeout, interval).Should(Succeed())
		})

		It("should not create a proxy-route if on the same zone as the API-Exposure", func() {
			By("Progressing the Approval resources")
			approvalReq := ProgressApprovalRequest(apiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(apiSubscription, approvalapi.ApprovalStateGranted, approvalReq)

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.Route).ToNot(BeNil())
				apiSubRoute := apiSubscription.Status.Route

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())
				apiExpRoute := apiExposure.Status.Route

				By("Checking if the route is the real-route")
				g.Expect(apiSubRoute).To(Equal(apiExpRoute))

			}, timeout, interval).Should(Succeed())
		})

		It("should create a ConsumeRoute", func() {
			By("Checking if the resource has the expected state")
			consumeRoute := &gatewayapi.ConsumeRoute{}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.Route).ToNot(BeNil())
				g.Expect(apiSubscription.Status.ConsumeRoute).ToNot(BeNil())

				err = k8sClient.Get(ctx, apiSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute.Spec.Route).To(Equal(*apiSubscription.Status.Route))
				g.Expect(consumeRoute.Spec.Security.M2M.Scopes[0]).To(Equal("scope1"))
				g.Expect(consumeRoute.Spec.Security.M2M.Scopes[1]).To(Equal("scope2"))

			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Meshing", func() {

		var apiSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Initializing the ApiSubscription")
			apiSubscription = NewApiSubscription(apiBasePath, otherZoneName, appName)

			By("Creating the Zone")
			otherZone = CreateZone(otherZoneName)

			By("Creating the Gateway")
			otherRealm := NewRealm(testEnvironment, otherZone.Name)
			err := k8sClient.Create(ctx, otherRealm)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create a proxy-route if on a different zone as the API-Exposure", func() {

			By("Creating the resource")
			err := k8sClient.Create(ctx, apiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				By("Checking the conditions")
				readyCondition := meta.FindStatusCondition(apiSubscription.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("ApprovalPending"))

			}, timeout, interval).Should(Succeed())

			By("Progressing the Approval resources")
			approvalReq := ProgressApprovalRequest(apiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(apiSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should create a proxy-route", func() {

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.Route).ToNot(BeNil())
				apiSubRoute := apiSubscription.Status.Route

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())
				apiExpRoute := apiExposure.Status.Route

				By("Checking if the route is the proxy-route")
				g.Expect(apiSubRoute).ToNot(Equal(apiExpRoute))

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiSubRoute.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(route.Spec.Upstreams[0].IssuerUrl).To(Equal("http://localhost:8080/auth/realms/test"))

			}, timeout, interval).Should(Succeed())
		})

	})

	Context("Prevent deletion of underlying Routes if multiple Api-Subscriptions exist", func() {

		appName := "my-second-app"
		var secondApiSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Initializing the second ApiSubscription")
			secondApiSubscription = NewApiSubscription(apiBasePath, otherZoneName, appName)
			By("Creating the Application")
			CreateApplication(appName)

			By("Setting up a second ApiSubscription")
			err := k8sClient.Create(ctx, secondApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the second resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApiSubscription), secondApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				By("Checking the conditions")
				readyCondition := meta.FindStatusCondition(secondApiSubscription.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("ApprovalPending"))

			}, timeout, interval).Should(Succeed())

			By("Progressing the Approval resources")
			approvalReq := ProgressApprovalRequest(secondApiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(secondApiSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should never remove the proxy-route if there is another active API-Subscription", func() {

			By("Ensuring that both Api-Subscription are actually ready and active")
			Eventually(func(g Gomega) {
				By("Checking the first ApiSubscription")
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.Route).ToNot(BeNil())

				By("Checking the second ApiSubscription")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApiSubscription), secondApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(secondApiSubscription.Status.Route).ToNot(BeNil())

			}, timeout, interval).Should(Succeed())

			By("Deleting the first ApiSubscription")
			err := k8sClient.Delete(ctx, apiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the route was not deleted")
			Eventually(func(g Gomega) {
				route := &gatewayapi.Route{}
				err := k8sClient.Get(ctx, apiSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

			}, timeout, interval).Should(Succeed())
		})
	})

})

var _ = Describe("Remote Organisation Flow", Ordered, func() {
	apiBasePath := "/apisubctrl/remotetest/v1"

	remoteZoneName := "remote-zone"
	consumerZoneName := "consumer-zone"
	appName := "remote-app"

	remoteOrgId := "esp"
	var apiSubscription *apiapi.ApiSubscription
	var remoteOrganisation *adminapi.RemoteOrganization

	var remoteApiSubscription *apiapi.RemoteApiSubscription

	Context("Remote Subscription Flow", func() {

		BeforeAll(func() {
			By("Creating the RemoteOrganisation")
			remoteOrganisation = CreateRemoteOrganisation(remoteOrgId, remoteZoneName)
			By("Creating the remote zone and its realms")
			zone := CreateZone(remoteZoneName)
			realm := NewRealm(testEnvironment, zone.Name)
			err := k8sClient.Create(ctx, realm)
			Expect(err).ToNot(HaveOccurred())
			CreateGatewayClient(zone)
			realm = NewRealm(remoteOrgId, zone.Name)
			err = k8sClient.Create(ctx, realm)
			Expect(err).ToNot(HaveOccurred())

			By("Creating the consumer zone and its realms")
			zone = CreateZone(consumerZoneName)
			realm = NewRealm(testEnvironment, zone.Name)
			err = k8sClient.Create(ctx, realm)
			Expect(err).ToNot(HaveOccurred())
			realm = NewRealm(remoteOrgId, zone.Name)
			err = k8sClient.Create(ctx, realm)
			Expect(err).ToNot(HaveOccurred())

			By("Creating the Application")
			CreateApplication(appName)

			By("Initializing the ApiSubscription")
			apiSubscription = NewApiSubscription(apiBasePath, remoteZoneName, appName)
			apiSubscription.Spec.Organization = remoteOrgId

			By("Creating the RemoteApiSubscription")
			remoteApiSubscription = &apiapi.RemoteApiSubscription{}
		})

		AfterAll(func() {
			By("Deleting the RemoteOrganisation")
			err := k8sClient.Delete(ctx, remoteOrganisation)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create a RemoteApiSubscription", func() {
			By("Creating the resource")
			err := k8sClient.Create(ctx, apiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.RemoteApiSubscription).ToNot(BeNil())

				err = k8sClient.Get(ctx, apiSubscription.Status.RemoteApiSubscription.K8s(), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(remoteApiSubscription.Spec.TargetOrganization).To(Equal(remoteOrgId))

			}, timeout, interval).Should(Succeed())

		})

		It("should progress when the RemoteApiSubscription has been approved and is ready", func() {
			By("Updating the RemoteApiSubscription")
			remoteApiSubscription.Status = apiapi.RemoteApiSubscriptionStatus{
				Approval: &apiapi.ApprovalInfo{
					ApprovalState: approvalapi.ApprovalStateGranted.String(),
					Message:       "Test-Message",
				},
				ApprovalRequest: &apiapi.ApprovalInfo{
					ApprovalState: approvalapi.ApprovalStateGranted.String(),
					Message:       "Test-Message",
				},
				GatewayUrl: "http://ger.gateway.es:8080/ger",
			}
			remoteApiSubscription.SetCondition(condition.NewReadyCondition("RemoteApiSubscriptionReady", "Trust me, I am ready"))

			err := k8sClient.Status().Update(ctx, remoteApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(apiSubscription.Status.Route).ToNot(BeNil())
				g.Expect(apiSubscription.Status.Route.Name).To(Equal("esp--apisubctrl-remotetest-v1")) // TODO: make this useable by multiple subs for same remote-api

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

			}, timeout, interval).Should(Succeed())
		})

		It("should re-use the Route created by the RemoteApiSubscription and not create a proxy-route", func() {

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				By("Checking that the route is the same as the RemoteApiSubscription")
				g.Expect(apiSubscription.Status.Route.Name).To(Equal("esp--apisubctrl-remotetest-v1"))
				g.Expect(apiSubscription.Status.Route.Namespace).To(Equal("test--remote-zone"))

			}, timeout, interval).Should(Succeed())

		})

		It("should create a proxy-route if the RemoteApiSubscription is in a different zone", func() {

			By("Changing the zone of the ApiSubscription")
			apiSubscription.Spec.Zone = types.ObjectRef{
				Name:      consumerZoneName,
				Namespace: testEnvironment,
			}

			err := k8sClient.Update(ctx, apiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {

				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(apiSubscription.Status.Route.Name).To(Equal("esp--apisubctrl-remotetest-v1"))
				g.Expect(apiSubscription.Status.Route.Namespace).To(Equal("test--consumer-zone"))

			}, timeout, interval).Should(Succeed())

		})
	})
})
