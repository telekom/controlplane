// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	secretmanager "github.com/telekom/controlplane/secret-manager/api"
	smfake "github.com/telekom/controlplane/secret-manager/api/fake"
	"github.com/telekom/controlplane/secret-manager/api/gen"

	mock "github.com/stretchr/testify/mock"
)

var _ = Describe("Rover Controller", Ordered, func() {

	const (
		resourceName = "test-resource"
		BasePath     = "/eni/api/v1"
		upstream     = "https://api.example.com"
		organization = "esp"
	)

	ctx := context.Background()
	var team *organizationv1.Team
	var mockSecretManager = &smfake.MockSecretManager{}

	typeNamespacedName := client.ObjectKey{
		Name:      resourceName,
		Namespace: teamNamespace,
	}

	BeforeAll(func() {
		By("Creating the environment Namespace")
		createNamespace(testEnvironment)

		By("Creating the Team")
		team = newTeam(teamName, group, testEnvironment, testEnvironment)
		err := k8sClient.Create(ctx, team)
		Expect(err).NotTo(HaveOccurred())

		By("Creating the team Namespace")
		createNamespace(teamNamespace)

		By("Mocking Secret Manager")
		secretmanager.API = func() secretmanager.SecretManager {
			return mockSecretManager
		}
	})

	AfterEach(func() {
		resource := &roverv1.Rover{}
		err := k8sClient.Get(ctx, typeNamespacedName, resource)
		Expect(err).NotTo(HaveOccurred())

		By("Cleanup the specific resource instance Rover")
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	AfterAll(func() {
		By("Cleanup the Team")
		Expect(k8sClient.Delete(ctx, team)).To(Succeed())
	})

	Context("Simple rover file with 1 exposure and 1 subscription", func() {

		It("should successfully reconcile the resource", func() {

			Spec := roverv1.RoverSpec{
				Zone: testEnvironment,
				Exposures: []roverv1.Exposure{
					{
						Api: &roverv1.ApiExposure{
							BasePath: BasePath,
							Upstreams: []roverv1.Upstream{
								{
									URL: upstream,
								},
							},
							Visibility: roverv1.VisibilityWorld,
							Approval: roverv1.Approval{
								Strategy: roverv1.ApprovalStrategyFourEyes,
							},
						},
					},
				},
				Subscriptions: []roverv1.Subscription{
					{
						Api: &roverv1.ApiSubscription{
							BasePath: BasePath,
						},
					},
				},
			}

			rover := createRover(resourceName, teamNamespace, testEnvironment, Spec)

			By("creating the custom resource for the Kind Rover")

			Expect(k8sClient.Create(ctx, rover)).To(Succeed())

			// mocking the call and response to secret manager
			mockedSecrets := make([]gen.ListSecretItem, 1)
			mockedSecrets[0] = gen.ListSecretItem{
				Id:   testEnvironment + ":" + team.Name + "::clientSecret:d174a02b9776",
				Name: "clientSecret",
			}
			mockSecretManager.EXPECT().UpsertApplication(mock.Anything, testEnvironment, team.Name, "test-resource").Return(mockedSecrets, nil)

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: teamNamespace,
				}, rover)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rover.Status.ApiExposures).To(HaveLen(1))
				g.Expect(rover.Status.ApiSubscriptions).To(HaveLen(1))

				application := &applicationv1.Application{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource",
					Namespace: teamNamespace,
				}, application)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(application.Spec.NeedsClient).To(Equal(true))
				g.Expect(application.Spec.NeedsConsumer).To(Equal(true))
				g.Expect(application.Spec.Secret).To(Not(BeEmpty()))
				g.Expect(application.Spec.Team).To(Equal(team.Name))
				g.Expect(application.Spec.TeamEmail).To(Equal(team.Spec.Email))
				g.Expect(application.Spec.Secret).To(Equal("$<test:eni--hyperion::clientSecret:d174a02b9776>"))

				apiExposure := &apiapi.ApiExposure{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiExposure)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiExposure.Spec.ApiBasePath).To(Equal("/eni/api/v1"))

				apiSubscription := &apiapi.ApiSubscription{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiSubscription)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiSubscription.Spec.ApiBasePath).To(Equal("/eni/api/v1"))

			}, timeout, interval).Should(Succeed())

			By("updating subcriptions")
			fetchedRover := &roverv1.Rover{}
			err := k8sClient.Get(ctx, typeNamespacedName, fetchedRover)
			Expect(err).NotTo(HaveOccurred())

			updateSubscriptions := []roverv1.Subscription{
				{
					Api: &roverv1.ApiSubscription{
						BasePath: BasePath,
					},
				},

				{
					Api: &roverv1.ApiSubscription{
						BasePath: "/eni/api/v2",
					},
				},
			}
			updateSpec := roverv1.RoverSpec{
				Zone: testEnvironment,
				Exposures: []roverv1.Exposure{
					{
						Api: &roverv1.ApiExposure{
							BasePath: BasePath,
							Upstreams: []roverv1.Upstream{
								{
									URL: upstream,
								},
							},

							Visibility: roverv1.VisibilityWorld,
							Approval: roverv1.Approval{
								Strategy: roverv1.ApprovalStrategyFourEyes,
							},
						},
					},
				},
				Subscriptions: updateSubscriptions,
			}

			fetchedRover.Spec = updateSpec

			// Update rover
			Expect(k8sClient.Update(ctx, fetchedRover)).To(Succeed())

			// fetch updated rover and validate subscriptions
			fetchedUpdatedRover := &roverv1.Rover{}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, fetchedUpdatedRover)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetchedUpdatedRover.Spec.Subscriptions).To(Equal(updateSubscriptions))

				apiSubscription := &apiapi.ApiSubscription{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v2",
					Namespace: teamNamespace,
				}, apiSubscription)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiSubscription.Spec.ApiBasePath).To(Equal("/eni/api/v2"))
			})

			By("updating exposures")
			updateExposures := []roverv1.Exposure{
				{
					Api: &roverv1.ApiExposure{
						BasePath: BasePath,
						Upstreams: []roverv1.Upstream{
							{
								URL: "https://my.new.upstream.de",
							},
						},

						Visibility: roverv1.VisibilityEnterprise,
						Approval: roverv1.Approval{
							Strategy: roverv1.ApprovalStrategySimple,
						},
					},
				},
			}

			updateSpec = roverv1.RoverSpec{
				Zone:          fetchedRover.Spec.Zone,
				Exposures:     updateExposures,
				Subscriptions: updateSubscriptions,
			}

			fetchedRover.Spec = updateSpec

			// Update rover
			Expect(k8sClient.Update(ctx, fetchedRover)).To(Succeed())

			// fetch updated rover and validate exposures
			fetchedUpdatedRover = &roverv1.Rover{}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, fetchedUpdatedRover)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetchedUpdatedRover.Spec.Exposures).To(Equal(updateExposures))

				apiExposure := &apiapi.ApiExposure{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiExposure)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiExposure.Spec.Visibility).To(Equal(apiapi.VisibilityEnterprise))
				g.Expect(apiExposure.Spec.Approval).To(Equal(apiapi.ApprovalStrategySimple))
				g.Expect(apiExposure.Spec.Upstreams[0].Url).To(Equal("https://my.new.upstream.de"))
			})

			By("deleting exposures and subscriptions")
			updateSpec = roverv1.RoverSpec{
				Zone:          testEnvironment,
				Exposures:     []roverv1.Exposure{},
				Subscriptions: []roverv1.Subscription{},
			}

			fetchedRover.Spec = updateSpec

			// Update rover
			Expect(k8sClient.Update(ctx, fetchedRover)).To(Succeed())

			// fetch updated rover and validate exposures
			fetchedUpdatedRover = &roverv1.Rover{}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, fetchedUpdatedRover)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetchedUpdatedRover.Spec.Exposures).To(BeEmpty())
				g.Expect(fetchedUpdatedRover.Spec.Subscriptions).To(BeEmpty())

				application := &applicationv1.Application{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource",
					Namespace: teamNamespace,
				}, application)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(application.Spec.NeedsClient).To(Equal(false))
				g.Expect(application.Spec.NeedsConsumer).To(Equal(false))

				apiExposure := &apiapi.ApiExposure{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiExposure)
				g.Expect(err).To(HaveOccurred())

				apiSubscription := &apiapi.ApiSubscription{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiSubscription)
				g.Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("Remote Organization subscription", func() {

		It("should successfully handle remote subscription and reconcile the resource", func() {

			Spec := roverv1.RoverSpec{
				Zone: testEnvironment,
				Subscriptions: []roverv1.Subscription{
					{
						Api: &roverv1.ApiSubscription{
							BasePath:     BasePath,
							Organization: organization,
						},
					},
				},
			}

			rover := createRover(resourceName, teamNamespace, testEnvironment, Spec)

			By("creating the custom resource for the Kind Rover")

			err := k8sClient.Get(ctx, typeNamespacedName, rover)

			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, rover)).To(Succeed())
			}

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: teamNamespace,
				}, rover)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rover.Status.ApiExposures).To(HaveLen(0))
				g.Expect(rover.Status.ApiSubscriptions).To(HaveLen(1))

				apiSubscription := &apiapi.ApiSubscription{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "esp--test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiSubscription)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiSubscription.Spec.ApiBasePath).To(Equal("/eni/api/v1"))
				g.Expect(apiSubscription.Spec.Requestor.Application.Name).To(Equal("test-resource"))
				g.Expect(apiSubscription.Spec.Organization).To(Equal("esp"))

				/*remoteApiSubscription := &apiapi.RemoteApiSubscription{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: testNamespace,
				}, remoteApiSubscription)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(remoteApiSubscription.Spec.ApiBasePath).To(Equal("/eni/api/v1"))
				g.Expect(remoteApiSubscription.Spec.Requester.Application).To(Equal("eni-api-v1"))
				g.Expect(remoteApiSubscription.Spec.SourceOrganization).To(Equal("esp"))
				g.Expect(remoteApiSubscription.Spec.TargetOrganization).To(Equal("de"))*/

			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Rover with OAuth2 scopes", func() {

		It("should successfully handle scopes and reconcile the resource", func() {

			Spec := roverv1.RoverSpec{
				Zone: testEnvironment,
				Subscriptions: []roverv1.Subscription{
					{
						Api: &roverv1.ApiSubscription{
							BasePath: BasePath,
							Security: &roverv1.SubscriberSecurity{
								M2M: &roverv1.SubscriberMachine2MachineAuthentication{

									Scopes: []string{"tardis:user:read"},
								},
							},
						},
					},
				},
			}

			rover := createRover(resourceName, teamNamespace, testEnvironment, Spec)

			By("creating the custom resource for the Kind Rover")

			Expect(k8sClient.Create(ctx, rover)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: teamNamespace,
				}, rover)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rover.Status.ApiSubscriptions).To(HaveLen(1))

				apiSubscription := &apiapi.ApiSubscription{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiSubscription)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiSubscription.Spec.Security.M2M.Scopes[0]).To(Equal("tardis:user:read"))

			}, timeout, interval).Should(Succeed())

		})
	})

	Context("Rover with ExternalIDPs", func() {

		It("should successfully handle scopes and reconcile the resource", func() {

			Spec := roverv1.RoverSpec{
				Zone: testEnvironment,
				Subscriptions: []roverv1.Subscription{
					{
						Api: &roverv1.ApiSubscription{
							BasePath: BasePath,
							Security: &roverv1.SubscriberSecurity{
								M2M: &roverv1.SubscriberMachine2MachineAuthentication{
									Client: &roverv1.OAuth2ClientCredentials{
										ClientId:     "clientID",
										ClientSecret: "******",
										Scopes:       []string{"eIDP:scope"},
									},
								},
							},
						},
					},
				},
				Exposures: []roverv1.Exposure{
					{
						Api: &roverv1.ApiExposure{
							BasePath: BasePath,
							Upstreams: []roverv1.Upstream{
								{
									URL: upstream,
								},
							},
							Visibility: roverv1.VisibilityWorld,
							Approval: roverv1.Approval{
								Strategy: roverv1.ApprovalStrategyFourEyes,
							},
							Security: &roverv1.Security{
								M2M: &roverv1.Machine2MachineAuthentication{
									ExternalIDP: &roverv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  "header",
										GrantType:     "client_credentials",
										Basic:         nil,
										Client: &roverv1.OAuth2ClientCredentials{
											ClientId:     "clientID",
											ClientSecret: "******",
											Scopes:       []string{"eIDP:scope"},
										},
									},
								},
							},
						},
					},
				},
			}

			rover := createRover(resourceName, teamNamespace, testEnvironment, Spec)

			By("creating the custom resource for the Kind Rover")

			Expect(k8sClient.Create(ctx, rover)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: teamNamespace,
				}, rover)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rover.Status.ApiSubscriptions).To(HaveLen(1))

				apiSubscription := &apiapi.ApiSubscription{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiSubscription)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiSubscription.Spec.Security.M2M.Client.ClientId).To(Equal("clientID"))
				g.Expect(apiSubscription.Spec.Security.M2M.Client.ClientSecret).To(Equal("******"))
				g.Expect(apiSubscription.Spec.Security.M2M.Client.Scopes[0]).To(Equal("eIDP:scope"))

				apiExposure := &apiapi.ApiExposure{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiExposure)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("clientID"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("******"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.Client.Scopes[0]).To(Equal("eIDP:scope"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.TokenRequest).To(Equal("header"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
			}, timeout, interval).Should(Succeed())

		})
	})

})
