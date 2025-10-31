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
			Secret:    "topsecret",
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
			Security: &apiapi.SubscriberSecurity{
				M2M: &apiapi.SubscriberMachine2MachineAuthentication{
					Client: &apiapi.OAuth2ClientCredentials{
						ClientId:     "client_id",
						ClientSecret: "******",
					},
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
		IssuerUrl: fmt.Sprintf("http://my-issuer.%s:8080/auth/realms/%s", zone.Name, testEnvironment),
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

	approval.Status.LastState = state
	k8sClient.Status().Update(ctx, approval)

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
				g.Expect(apiSubRoute.Name).To(Equal(apiExpRoute.Name))
				g.Expect(apiSubRoute.Namespace).To(Equal(apiExpRoute.Namespace))

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
				g.Expect(consumeRoute.Spec.Security.M2M.Scopes[0]).To(Equal("scope1"))
				g.Expect(consumeRoute.Spec.Security.M2M.Scopes[1]).To(Equal("scope2"))
				g.Expect(consumeRoute.Spec.Security.M2M.Scopes).To(ConsistOf("scope1", "scope2"))

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

				g.Expect(route.Spec.Upstreams[0].IssuerUrl).To(Equal("http://my-issuer.apisub-test:8080/auth/realms/test"))

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

	Context("Gateway Features", Ordered, func() {
		Context("oauth2 configuration to consumer route", func() {
			var apiSubscription *apiapi.ApiSubscription

			BeforeEach(func() {
				apiSubscription = NewApiSubscription(apiBasePath, otherZoneName, appName)
				apiSubscription.ObjectMeta.Name = apiSubscription.Name + "-security"
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, apiSubscription)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should apply those configs to ConsumeRoute", func() {
				By("applying oauth2 security to the ApiSubscription")
				apiSubscription.Spec.Security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{
					Client: &apiapi.OAuth2ClientCredentials{
						ClientId:     "custom-client-id",
						ClientSecret: "******",
					},
					Scopes: []string{"scope1", "scope2"},
				}
				err := k8sClient.Create(ctx, apiSubscription)
				Expect(err).ToNot(HaveOccurred())

				By("Checking if the resource the approval is pending")
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
					g.Expect(err).ToNot(HaveOccurred())
					By("Checking the conditions")
					processingCondition := meta.FindStatusCondition(apiSubscription.Status.Conditions, condition.ConditionTypeProcessing)
					g.Expect(processingCondition).ToNot(BeNil())
					g.Expect(processingCondition.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(processingCondition.Reason).To(Equal("ApprovalPending"))
				}, timeout, interval).Should(Succeed())

				By("Progressing the Approval resources")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				Expect(err).ToNot(HaveOccurred())
				approvalReq := ProgressApprovalRequest(apiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
				ProgressApproval(apiSubscription, approvalapi.ApprovalStateGranted, approvalReq)
				Expect(err).ToNot(HaveOccurred())

				By("Checking if the resource has the expected state")
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(apiSubscription.Status.Route).ToNot(BeNil())
					g.Expect(apiSubscription.Status.ConsumeRoute).ToNot(BeNil())
					By("Checking the conditions")
					readyCondition := meta.FindStatusCondition(apiSubscription.Status.Conditions, condition.ConditionTypeReady)
					g.Expect(readyCondition).ToNot(BeNil())
					g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(readyCondition.Reason).To(Equal("Provisioned"))

					consumeRoute := &gatewayapi.ConsumeRoute{}
					err = k8sClient.Get(ctx, apiSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(consumeRoute.Spec.Security.M2M.Client.ClientId).To(Equal("custom-client-id"))
					g.Expect(consumeRoute.Spec.Security.M2M.Client.ClientSecret).To(Equal("******"))
					g.Expect(consumeRoute.Spec.Security.M2M.Scopes).To(Equal([]string{"scope1", "scope2"}))
					g.Expect(consumeRoute.Spec.Route).To(Equal(*apiSubscription.Status.Route))

				}, timeout, interval).Should(Succeed())
			})
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

var _ = Describe("ApiSubscription Controller with failover scenario", Ordered, func() {
	// Scenario 1:
	// ApiSubscription is in the same zone as the ApiExposure failover zone
	// Normal-Flow: consumerZone -> providerZone -> providerApi
	// Failover-Flow: consumerZone == providerFailoverZone -> providerApi
	// Scenario 2:
	// ApiSubscription is in a different zone as the ApiExposure failover zone
	// Normal-Flow: consumerZone -> providerZone -> providerApi
	// Failover-Flow: consumerZone -> providerFailoverZone -> providerApi
	// Scenario 3:
	// ApiSubscription with multiple failover zones configured
	// Tests creation of multiple failover routes and consume routes
	// Scenario 4:
	// ApiSubscription in different zone as ApiExposure and ApiExposure failover zones
	// ApiSubscription failover zone is the same as the ApiExposure zone

	var apiBasePath = "/apisub/failovertest/v1"

	// Provider side
	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure

	// Provider/Exposure zone
	var providerZoneName = "provider-zone"
	var providerZone *adminapi.Zone

	// Failover zone for Provider
	var failoverZoneName = "apisub-failover-zone"
	var failoverZone *adminapi.Zone

	// Consumer side
	var appName = "failover-test-app"
	var application *applicationapi.Application

	BeforeAll(func() {
		By("Creating the provider zone")
		providerZone = CreateZone(providerZoneName)
		CreateGatewayClient(providerZone)

		By("Creating the failover zone")
		failoverZone = CreateZone(failoverZoneName)
		CreateGatewayClient(failoverZone)

		By("Creating the provider Realm")
		providerRealm := NewRealm(testEnvironment, providerZone.Name)
		err := k8sClient.Create(ctx, providerRealm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the failover Realm")
		failoverRealm := NewRealm(testEnvironment, failoverZone.Name)
		err = k8sClient.Create(ctx, failoverRealm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the Application")
		application = CreateApplication(appName)

		By("Initializing the API")
		api = NewApi(apiBasePath)
		err = k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		By("Creating APIExposure with failover configuration")
		apiExposure = NewApiExposure(apiBasePath, providerZoneName)
		apiExposure.Spec.Traffic = apiapi.Traffic{
			Failover: &apiapi.Failover{
				Zones: []types.ObjectRef{
					{
						Name:      failoverZone.Name,
						Namespace: failoverZone.Namespace,
					},
				},
			},
		}
		err = k8sClient.Create(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		By("Checking if APIExposure is created with proper failover configuration")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(apiExposure.Status.Active).To(BeTrue())
			g.Expect(apiExposure.Status.Route).ToNot(BeNil())
			g.Expect(apiExposure.Status.FailoverRoute).ToNot(BeNil())
		}, timeout, interval).Should(Succeed())
	})

	Context("Same Zone as ApiExposure Failover Zone", func() {
		var sameZoneSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating ApiSubscription in the failover zone")
			sameZoneSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			sameZoneSubscription.Name = "failover-same-zone-subscription"
			sameZoneSubscription.Spec.Zone = types.ObjectRef{
				Name:      failoverZoneName,
				Namespace: testEnvironment,
			}
			err := k8sClient.Create(ctx, sameZoneSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSubscription), sameZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(sameZoneSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(sameZoneSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should reuse the Proxy-Route created as secondary-route by ApiExposure", func() {
			By("Checking route configuration")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSubscription), sameZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, sameZoneSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify route has proper downstream configuration
				g.Expect(route.Spec.Downstreams[0].Url()).To(Equal("https://my-gateway.apisub-failover-zone:8080/apisub/failovertest/v1"))
				g.Expect(route.Spec.Downstreams[0].IssuerUrl).To(Equal("http://my-issuer.apisub-failover-zone:8080/auth/realms/test"))

				// Verify route has proper upstream configuration pointing to provider zone
				g.Expect(route.Spec.Upstreams[0].Url()).To(Equal("http://my-gateway.provider-zone:8080/apisub/failovertest/v1"))
				g.Expect(route.Spec.Upstreams[0].IssuerUrl).To(Equal("http://my-issuer.provider-zone:8080/auth/realms/test"))

				// Verify route has proper failover configuration pointing to provider API
				g.Expect(route.Spec.Traffic.Failover).ToNot(BeNil())
				g.Expect(route.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))
				g.Expect(route.Spec.Traffic.Failover.Upstreams[0].IssuerUrl).To(Equal(""))
				g.Expect(route.Spec.Traffic.Failover.Upstreams[0].Url()).To(Equal("http://my-provider-api:8080/api/v1"))

			}, timeout, interval).Should(Succeed())
		})

		It("should create a consume route for the ApiSubscription", func() {
			By("Checking consume route creation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSubscription), sameZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneSubscription.Status.ConsumeRoute).ToNot(BeNil())

				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, sameZoneSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute.Spec.Route).To(Equal(*sameZoneSubscription.Status.Route))
				g.Expect(consumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Different Zone than ApiExposure Failover Zone", func() {
		var differentZoneName = "different-zone"
		var differentZone *adminapi.Zone
		var differentZoneSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating a different zone")
			differentZone = CreateZone(differentZoneName)
			CreateGatewayClient(differentZone)

			By("Creating the Realm for different zone")
			differentRealm := NewRealm(testEnvironment, differentZone.Name)
			err := k8sClient.Create(ctx, differentRealm)
			Expect(err).ToNot(HaveOccurred())

			By("Creating ApiSubscription in a different zone")
			differentZoneSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			differentZoneSubscription.Name = "failover-different-zone-subscription"
			differentZoneSubscription.Spec.Zone = types.ObjectRef{
				Name:      differentZoneName,
				Namespace: testEnvironment,
			}
			err = k8sClient.Create(ctx, differentZoneSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(differentZoneSubscription), differentZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(differentZoneSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(differentZoneSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(differentZoneSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should create a proxy route with failover that points to the Api-Provider failover zone", func() {
			By("Checking route configuration")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(differentZoneSubscription), differentZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(differentZoneSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, differentZoneSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify route has proper downstream configuration
				g.Expect(route.Spec.Downstreams[0].Url()).To(Equal("https://my-gateway.different-zone:8080/apisub/failovertest/v1"))
				g.Expect(route.Spec.Downstreams[0].IssuerUrl).To(Equal("http://my-issuer.different-zone:8080/auth/realms/test"))

				// Verify route has proper upstream configuration pointing to provider zone
				g.Expect(route.Spec.Upstreams[0].Url()).To(Equal("http://my-gateway.provider-zone:8080/apisub/failovertest/v1"))
				g.Expect(route.Spec.Upstreams[0].IssuerUrl).To(Equal("http://my-issuer.provider-zone:8080/auth/realms/test"))

				// Verify route has proper failover configuration pointing to provider failover zone
				g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
				g.Expect(route.Spec.Traffic.Failover).ToNot(BeNil())
				g.Expect(route.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))
				g.Expect(route.Spec.Traffic.Failover.Upstreams[0].IssuerUrl).To(Equal("http://my-issuer.apisub-failover-zone:8080/auth/realms/test"))
				g.Expect(route.Spec.Traffic.Failover.Upstreams[0].Url()).To(Equal("http://my-gateway.apisub-failover-zone:8080/apisub/failovertest/v1"))

				g.Expect(route.Labels[config.BuildLabelKey("failover.zone")]).To(Equal(labelutil.NormalizeValue(failoverZone.Name)))
			}, timeout, interval).Should(Succeed())
		})

		It("should create a consume route for the ApiSubscription", func() {
			By("Checking consume route creation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(differentZoneSubscription), differentZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(differentZoneSubscription.Status.ConsumeRoute).ToNot(BeNil())

				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, differentZoneSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute.Spec.Route).To(Equal(*differentZoneSubscription.Status.Route))
				g.Expect(consumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("ApiSubscription with Multiple Failover Zones", func() {
		var multiFailoverZoneName1 = "multi-failover-zone1"
		var multiFailoverZoneName2 = "multi-failover-zone2"
		var multiFailoverZone1, multiFailoverZone2 *adminapi.Zone
		var multiFailoverSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating multiple failover zones")
			multiFailoverZone1 = CreateZone(multiFailoverZoneName1)
			multiFailoverZone2 = CreateZone(multiFailoverZoneName2)
			CreateGatewayClient(multiFailoverZone1)
			CreateGatewayClient(multiFailoverZone2)

			By("Creating the Realms for failover zones")
			realm1 := NewRealm(testEnvironment, multiFailoverZone1.Name)
			realm2 := NewRealm(testEnvironment, multiFailoverZone2.Name)
			err := k8sClient.Create(ctx, realm1)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Create(ctx, realm2)
			Expect(err).ToNot(HaveOccurred())

			By("Creating ApiSubscription with multiple failover zones")
			multiFailoverSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			multiFailoverSubscription.Name = "multi-failover-zone-subscription"
			multiFailoverSubscription.Spec.Zone = types.ObjectRef{
				Name:      "different-zone",
				Namespace: testEnvironment,
			}
			// Configure multiple failover zones
			multiFailoverSubscription.Spec.Traffic = apiapi.SubscriberTraffic{
				Failover: &apiapi.Failover{
					Zones: []types.ObjectRef{
						{
							Name:      multiFailoverZoneName1,
							Namespace: testEnvironment,
						},
						{
							Name:      multiFailoverZoneName2,
							Namespace: testEnvironment,
						},
					},
				},
			}
			err = k8sClient.Create(ctx, multiFailoverSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(multiFailoverSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(multiFailoverSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(multiFailoverSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should create a proxy route for the subscription zone", func() {
			By("Checking route configuration")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(multiFailoverSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify proxy route has proper downstream configuration
				g.Expect(route.Spec.Downstreams[0].Url()).To(Equal("https://my-gateway.different-zone:8080/apisub/failovertest/v1"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create a consume route for the proxy route", func() {
			By("Checking consume route creation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(multiFailoverSubscription.Status.ConsumeRoute).ToNot(BeNil())

				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute.Spec.Route).To(Equal(*multiFailoverSubscription.Status.Route))
			}, timeout, interval).Should(Succeed())
		})

		It("should create failover routes for each configured failover zone", func() {
			By("Checking failover routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that two failover routes are created
				g.Expect(multiFailoverSubscription.Status.FailoverRoutes).To(HaveLen(2))

				// Check first failover route
				route1 := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.FailoverRoutes[0].K8s(), route1)
				g.Expect(err).ToNot(HaveOccurred())

				// Check second failover route
				route2 := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.FailoverRoutes[1].K8s(), route2)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that the routes were created in the correct zones
				g.Expect(route1.Namespace).To(Equal("test--multi-failover-zone1"))
				g.Expect(route2.Namespace).To(Equal("test--multi-failover-zone2"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create consume routes for each failover route", func() {
			By("Checking failover consume routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that two failover consume routes are created
				g.Expect(multiFailoverSubscription.Status.FailoverConsumeRoutes).To(HaveLen(2))

				// Check first failover consume route
				consumeRoute1 := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.FailoverConsumeRoutes[0].K8s(), consumeRoute1)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute1.Spec.Route).To(Equal(multiFailoverSubscription.Status.FailoverRoutes[0]))
				g.Expect(consumeRoute1.Spec.ConsumerName).To(Equal(application.Status.ClientId))

				// Check second failover consume route
				consumeRoute2 := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.FailoverConsumeRoutes[1].K8s(), consumeRoute2)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute2.Spec.Route).To(Equal(multiFailoverSubscription.Status.FailoverRoutes[1]))
				g.Expect(consumeRoute2.Spec.ConsumerName).To(Equal(application.Status.ClientId))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("ApiSubscription with Failover Zone same as ApiExposure Zone", func() {
		var differentZoneName = "another-different-zone"
		var differentZone *adminapi.Zone
		var subscription *apiapi.ApiSubscription

		var appName = "same-sub-failover-zone-exp-zone"
		var application *applicationapi.Application

		BeforeAll(func() {
			By("Creating a different zone")
			differentZone = CreateZone(differentZoneName)
			CreateGatewayClient(differentZone)

			By("Creating the Application")
			application = CreateApplication(appName)

			By("Creating the Realm for different zone")
			realm := NewRealm(testEnvironment, differentZone.Name)
			err := k8sClient.Create(ctx, realm)
			Expect(err).ToNot(HaveOccurred())

			By("Creating ApiSubscription in different zone with provider zone as failover")
			subscription = NewApiSubscription(apiBasePath, differentZoneName, appName)
			// Configure failover zone to be the same as ApiExposure zone
			subscription.Spec.Traffic = apiapi.SubscriberTraffic{
				Failover: &apiapi.Failover{
					Zones: []types.ObjectRef{
						apiExposure.Spec.Zone, // Failover Zone is same zone as ApiExposure
					},
				},
			}
			err = k8sClient.Create(ctx, subscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(subscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(subscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should NOT create a failover route in the provider zone", func() {
			By("Checking failover routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(subscription.Status.FailoverRoutes).To(HaveLen(1))
			}, timeout, interval).Should(Succeed())
		})

		It("should create consume routes for both proxy and failover routes", func() {
			By("Checking consume routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Check proxy consume route
				g.Expect(subscription.Status.ConsumeRoute).ToNot(BeNil())
				proxyConsumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, subscription.Status.ConsumeRoute.K8s(), proxyConsumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(proxyConsumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))
				g.Expect(proxyConsumeRoute.Spec.Route).To(Equal(*subscription.Status.Route)) // should be the proxy route

				// Check failover consume route
				g.Expect(subscription.Status.FailoverConsumeRoutes).To(HaveLen(1))
				failoverConsumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, subscription.Status.FailoverConsumeRoutes[0].K8s(), failoverConsumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(failoverConsumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))

				// The failover ConsumeRoute must be the Route created by the ApiExposure
				g.Expect(failoverConsumeRoute.Spec.Route.Name).To(Equal(apiExposure.Status.Route.Name))
				g.Expect(failoverConsumeRoute.Spec.Route.Namespace).To(Equal(apiExposure.Status.Route.Namespace))
			}, timeout, interval).Should(Succeed())
		})
	})
})
