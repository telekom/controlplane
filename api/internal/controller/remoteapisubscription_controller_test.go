// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateRemoteOrganisation(orgId, zoneName string) *adminapi.RemoteOrganization {
	remoteOrg := &adminapi.RemoteOrganization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orgId,
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminapi.RemoteOrganizationSpec{
			Id:           orgId,
			Url:          "https://gateway.orgId.com/controlplane/v1",
			ClientId:     "ger-client",
			ClientSecret: "topsecret",
			IssuerUrl:    "https://gateway.orgId.com/auth/realms/test",
			Zone: types.ObjectRef{
				Name:      zoneName,
				Namespace: testEnvironment,
			},
		},
	}

	err := k8sClient.Create(ctx, remoteOrg)
	Expect(err).ToNot(HaveOccurred())

	remoteOrg.Status.Namespace = testEnvironment + "--" + orgId
	err = k8sClient.Status().Update(ctx, remoteOrg)
	Expect(err).ToNot(HaveOccurred())

	CreateNamespace(remoteOrg.Status.Namespace)
	return remoteOrg
}

func NewRemoteApiSubscription(apiBasePath, appName string) *apiapi.RemoteApiSubscription {
	return &apiapi.RemoteApiSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", appName, labelutil.NormalizeValue(apiBasePath)),
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: apiapi.RemoteApiSubscriptionSpec{
			ApiBasePath:        apiBasePath,
			TargetOrganization: "esp",
			SourceOrganization: "ger",
			Security: &apiapi.SubscriberSecurity{
				M2M: &apiapi.SubscriberMachine2MachineAuthentication{
					Scopes: []string{"scope1", "scope2"},
				},
			},
			Requester: apiapi.RemoteRequester{
				Application: appName,
				Team: apiapi.RemoteTeam{
					Name:  "test-team",
					Email: "team@test.com",
				},
			},
		},
	}
}

var _ = Describe("RemoteApiSubscription Controller - Provider Scenario", Ordered, func() {
	var apiBasePath = "/remoteapisubctrl/testprov/v1"
	var zoneName = "remoteapisub-test"
	var remoteOrgId = "ger"
	var appName = "my-remote-test-app"

	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure
	var zone *adminapi.Zone
	var remoteZone *adminapi.Zone
	var remoteOrg *adminapi.RemoteOrganization

	var remoteApiSubscription *apiapi.RemoteApiSubscription

	BeforeAll(func() {
		By("Initializing the API, APIExposure and RemoteApiSubscription")
		api = NewApi(apiBasePath)
		apiExposure = NewApiExposure(apiBasePath, zoneName)
		remoteApiSubscription = NewRemoteApiSubscription(apiBasePath, appName)

		By("Creating the normal Zone")
		zone = CreateZone(zoneName)
		realm := NewRealm(testEnvironment, zone.Name)
		err := k8sClient.Create(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
		CreateGatewayClient(zone)

		By("Creating the remote Zone")
		remoteZone = CreateZone(remoteOrgId + "-" + zoneName)
		remoteRealm := NewRealm(testEnvironment, remoteZone.Name)
		remoteRealm.Spec.Url = "https://ger.gateway.es"
		err = k8sClient.Create(ctx, remoteRealm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the RemoteOrganization")
		remoteOrg = CreateRemoteOrganisation(remoteOrgId, zoneName)
	})

	AfterAll(func() {
		By("Deleting the RemoteOrganization")
		err := k8sClient.Delete(ctx, remoteOrg)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("We are the target of the remote API subscription", func() {

		It("should block until an API is registered", func() {
			By("Creating the resource")
			err := k8sClient.Create(ctx, remoteApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				processingCondition := meta.FindStatusCondition(remoteApiSubscription.Status.Conditions, condition.ConditionTypeProcessing)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(processingCondition.Reason).To(Equal("Blocked"))

			}, timeout, interval).Should(Succeed())

			// TODO: test if syncClient was called

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
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				readyCondition := meta.FindStatusCondition(remoteApiSubscription.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("ApprovalPending"))

			}, timeout, interval).Should(Succeed())

			// TODO: test if syncClient was called

		})

		It("should create the application", func() {
			By("Checking if the application has been created")
			Eventually(func(g Gomega) {
				application := &applicationapi.Application{}
				err := k8sClient.Get(ctx, remoteApiSubscription.Status.Application.K8s(), application)
				g.Expect(err).ToNot(HaveOccurred())

				By("Checking if the application has the expected state")
				g.Expect(application.Spec.NeedsClient).To(BeFalse())
				g.Expect(application.Spec.NeedsConsumer).To(BeTrue())
				g.Expect(application.Spec.Zone.Name).To(Equal(remoteZone.Name))

			}, timeout, interval).Should(Succeed())
		})

		It("should create the api-subscription", func() {
			By("Checking if the api-subscription has been created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(remoteApiSubscription.Status.ApiSubscription).ToNot(BeNil())

				apiSubscription := &apiapi.ApiSubscription{}
				err = k8sClient.Get(ctx, remoteApiSubscription.Status.ApiSubscription.K8s(), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				By("Checking if the api-subscription has the expected state")
				g.Expect(apiSubscription.Spec.ApiBasePath).To(Equal(apiBasePath))
				g.Expect(apiSubscription.Spec.Requestor.Application.Name).To(Equal(appName))
				g.Expect(apiSubscription.Spec.Requestor.Application.Namespace).To(Equal(testNamespace))
				g.Expect(apiSubscription.Spec.Organization).To(Equal(""))
				g.Expect(apiSubscription.Spec.Zone.Name).To(Equal(remoteZone.Name))

				By("Checking the conditions")
				readyCondition := meta.FindStatusCondition(apiSubscription.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("ApprovalPending"))

			}, timeout, interval).Should(Succeed())
		})

		It("should process when the Approval is granted", func() {
			apiSubscription := &apiapi.ApiSubscription{}
			err := k8sClient.Get(ctx, remoteApiSubscription.Status.ApiSubscription.K8s(), apiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Progressing the Approval resources")
			approvalReq := ProgressApprovalRequest(apiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(apiSubscription, approvalapi.ApprovalStateGranted, approvalReq)

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(remoteApiSubscription.Status.ApiSubscription).ToNot(BeNil())

				By("checking that the route-info is filled")
				g.Expect(remoteApiSubscription.Status.GatewayUrl).To(Equal("https://ger.gateway.es/remoteapisubctrl/testprov/v1"))

			}, timeout, interval).Should(Succeed())
		})

	})
})

var _ = Describe("RemoteApiSubscription Controller - Consumer Scenario", Ordered, func() {
	var apiBasePath = "/remoteapisubctrl/testcons/v1"
	var zoneName = "remoteapisub-test-cons"
	var appName = "my-remote-test-cons-app"
	var remoteOrgId = "pol"

	var zone *adminapi.Zone
	var remoteOrg *adminapi.RemoteOrganization

	var remoteApiSubscription *apiapi.RemoteApiSubscription

	BeforeAll(func() {
		By("Initializing the RemoteApiSubscription")
		remoteApiSubscription = NewRemoteApiSubscription(apiBasePath, appName)
		remoteApiSubscription.Spec.TargetOrganization = remoteOrgId

		By("Creating the Zone")
		zone = CreateZone(zoneName)

		By("Creating the Realm")
		realm := NewRealm(remoteOrgId, zone.Name)
		err := k8sClient.Create(ctx, realm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the RemoteOrganization")
		remoteOrg = CreateRemoteOrganisation(remoteOrgId, zoneName)

	})

	AfterAll(func() {
		By("Deleting the RemoteOrganization")
		err := k8sClient.Delete(ctx, remoteOrg)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("We are the consumer of the remote API subscription", func() {

		It("should send the RemoteApiSubscription to the target", func() {
			By("Creating the resource")
			err := k8sClient.Create(ctx, remoteApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			// TODO: test if syncClient was called

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				processingCondition := meta.FindStatusCondition(remoteApiSubscription.Status.Conditions, condition.ConditionTypeProcessing)
				g.Expect(processingCondition).ToNot(BeNil())
				g.Expect(processingCondition.Status).To(Equal(metav1.ConditionUnknown))

			}, timeout, interval).Should(Succeed())

		})

		It("should progress the RemoteApiSubscription if the sync was successful and approved", func() {
			By("Updating the resource")
			remoteApiSubscription.Status = apiapi.RemoteApiSubscriptionStatus{
				Approval: &apiapi.ApprovalInfo{
					ApprovalState: approvalapi.ApprovalStateGranted.String(),
					Message:       "Test-Message",
				},
				ApprovalRequest: &apiapi.ApprovalInfo{
					ApprovalState: approvalapi.ApprovalStateGranted.String(),
					Message:       "Test-Message",
				},
				GatewayUrl: "http://ger.gateway.pol:8080/ger",
			}
			remoteApiSubscription.SetCondition(condition.NewReadyCondition("RemoteApiSubscriptionReady", "Trust me, I am ready"))

			err := k8sClient.Status().Update(ctx, remoteApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(remoteApiSubscription.Status.Route).ToNot(BeNil())

			}, timeout, interval).Should(Succeed())
		})
	})

	It("should delete the RemoteApiSubscription and its Route", func() {
		By("Deleting the resource")
		err := k8sClient.Delete(ctx, remoteApiSubscription)
		Expect(err).ToNot(HaveOccurred())

		By("Checking if the resource has been deleted")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(remoteApiSubscription), remoteApiSubscription)
			g.Expect(err).To(HaveOccurred())

			err = k8sClient.Get(ctx, remoteApiSubscription.Status.Route.K8s(), &gatewayapi.Route{})
			g.Expect(err).To(HaveOccurred())

			// TODO: test if syncClient was called

		}, timeout, interval).Should(Succeed())
	})

})
