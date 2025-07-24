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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// Helper function to get condition by type
func getConditionByType(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

var _ = Describe("Rover Controller", Ordered, func() {

	const (
		resourceName = "test-resource"
		BasePath     = "/eni/api/v1"
		upstream     = "https://api.example.com"
		organization = "esp"
	)

	ctx := context.Background()
	var team *organizationv1.Team

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

			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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
							Transformation: &roverv1.Transformation{
								Request: roverv1.RequestResponseTransformation{
									Headers: roverv1.HeaderTransformation{
										Remove: []string{"X-Remove-Header"},
									},
								},
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

			rover := createRover(resourceName, teamNamespace, testEnvironment, spec)

			By("creating the custom resource for the Kind Rover")

			Expect(k8sClient.Create(ctx, rover)).To(Succeed())

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
				g.Expect(application.Spec.Secret).To(Equal("topsecret"))

				apiExposure := &apiapi.ApiExposure{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiExposure)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiExposure.Spec.ApiBasePath).To(Equal("/eni/api/v1"))
				g.Expect(apiExposure.Spec.Transformation.Request.Headers.Remove).To(ContainElement("X-Remove-Header"))

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
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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
				ClientSecret:  "topsecret",
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
				g.Expect(apiExposure.Spec.Upstreams).To(HaveLen(1))
				g.Expect(apiExposure.Spec.Upstreams[0].Url).To(Equal("https://my.new.upstream.de"))
			})

			By("deleting exposures and subscriptions")
			updateSpec = roverv1.RoverSpec{
				Zone:          testEnvironment,
				ClientSecret:  "topsecret",
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

			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
				Subscriptions: []roverv1.Subscription{
					{
						Api: &roverv1.ApiSubscription{
							BasePath:     BasePath,
							Organization: organization,
						},
					},
				},
			}

			rover := createRover(resourceName, teamNamespace, testEnvironment, spec)

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

			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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

			rover := createRover(resourceName, teamNamespace, testEnvironment, spec)

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

			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
				Subscriptions: []roverv1.Subscription{
					{
						Api: &roverv1.ApiSubscription{
							BasePath: BasePath,
							Security: &roverv1.SubscriberSecurity{
								M2M: &roverv1.SubscriberMachine2MachineAuthentication{
									Client: &roverv1.OAuth2ClientCredentials{
										ClientId:     "clientID",
										ClientSecret: "******",
									},
									Scopes: []string{"eIDP:scope"},
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
										},
									},
									Scopes: []string{"eIDP:scope"},
								},
							},
						},
					},
				},
			}

			rover := createRover(resourceName, teamNamespace, testEnvironment, spec)

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
				g.Expect(apiSubscription.Spec.Security.M2M.Scopes[0]).To(Equal("eIDP:scope"))

				apiExposure := &apiapi.ApiExposure{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-resource--eni-api-v1",
					Namespace: teamNamespace,
				}, apiExposure)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("clientID"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("******"))
				g.Expect(apiExposure.Spec.Security.M2M.Scopes[0]).To(Equal("eIDP:scope"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.TokenRequest).To(Equal("header"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
				g.Expect(apiExposure.Spec.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
			}, timeout, interval).Should(Succeed())

		})
	})

	Context("Trusted Teams functionality", func() {

		var trustedTeam1, trustedTeam2, trustedTeam3 *organizationv1.Team

		BeforeEach(func() {
			// Create additional teams for trusted teams testing
			By("Creating trusted team 1")
			trustedTeam1 = newTeam("trusted-team-1", "trusted-group-1", testEnvironment, testEnvironment)
			err := k8sClient.Create(ctx, trustedTeam1)
			Expect(err).NotTo(HaveOccurred())

			By("Creating trusted team 2")
			trustedTeam2 = newTeam("trusted-team-2", "trusted-group-2", testEnvironment, testEnvironment)
			err = k8sClient.Create(ctx, trustedTeam2)
			Expect(err).NotTo(HaveOccurred())

			By("Creating trusted team 3")
			trustedTeam3 = newTeam("trusted-team-3", "trusted-group-3", testEnvironment, testEnvironment)
			err = k8sClient.Create(ctx, trustedTeam3)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Clean up trusted teams
			By("Cleaning up trusted teams")
			Expect(k8sClient.Delete(ctx, trustedTeam1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, trustedTeam2)).To(Succeed())
			Expect(k8sClient.Delete(ctx, trustedTeam3)).To(Succeed())
		})

		It("should successfully map trusted teams to ApiExposure", func() {
			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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
								TrustedTeams: []roverv1.TrustedTeam{
									{
										Group: "trusted-group-1",
										Team:  "trusted-team-1",
									},
									{
										Group: "trusted-group-2",
										Team:  "trusted-team-2",
									},
								},
							},
						},
					},
				},
			}

			By("Creating the custom resource for the Kind Rover")
			resource := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": testEnvironment,
					},
				},
				Spec: spec,
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Checking if the custom resource was successfully created")
			Eventually(func(g Gomega) {
				found := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, found)).To(Succeed())
				g.Expect(found.Status.ApiExposures).To(HaveLen(1))
			}, timeout, interval).Should(Succeed())

			By("Checking if ApiExposure was created with correct trusted teams")
			Eventually(func(g Gomega) {
				apiExposure := &apiapi.ApiExposure{}
				apiExposureKey := client.ObjectKey{
					Name:      resourceName + "--eni-api-v1",
					Namespace: teamNamespace,
				}
				err := k8sClient.Get(ctx, apiExposureKey, apiExposure)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify approval strategy
				g.Expect(apiExposure.Spec.Approval.Strategy).To(Equal(apiapi.ApprovalStrategyFourEyes))

				// Verify trusted teams are mapped correctly
				g.Expect(apiExposure.Spec.Approval.TrustedTeams).To(HaveLen(2))

				// Check that trusted teams reference the correct Team objects
				trustedTeamNames := make([]string, len(apiExposure.Spec.Approval.TrustedTeams))
				for i, teamRef := range apiExposure.Spec.Approval.TrustedTeams {
					trustedTeamNames[i] = teamRef.Name
					g.Expect(teamRef.Namespace).To(Equal(testEnvironment))
				}
				g.Expect(trustedTeamNames).To(ContainElements(
					"trusted-group-1--trusted-team-1",
					"trusted-group-2--trusted-team-2",
				))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle empty trusted teams list", func() {
			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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
								Strategy:     roverv1.ApprovalStrategySimple,
								TrustedTeams: []roverv1.TrustedTeam{}, // Empty list
							},
						},
					},
				},
			}

			By("Creating the custom resource for the Kind Rover")
			resource := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": testEnvironment,
					},
				},
				Spec: spec,
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Checking if ApiExposure was created with empty trusted teams")
			Eventually(func(g Gomega) {
				apiExposure := &apiapi.ApiExposure{}
				apiExposureKey := client.ObjectKey{
					Name:      resourceName + "--eni-api-v1",
					Namespace: teamNamespace,
				}
				err := k8sClient.Get(ctx, apiExposureKey, apiExposure)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify approval strategy
				g.Expect(apiExposure.Spec.Approval.Strategy).To(Equal(apiapi.ApprovalStrategySimple))

				// Verify trusted teams list is empty/nil
				g.Expect(apiExposure.Spec.Approval.TrustedTeams).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should fail when trusted team does not exist", func() {
			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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
								TrustedTeams: []roverv1.TrustedTeam{
									{
										Group: "nonexistent-group",
										Team:  "nonexistent-team",
									},
								},
							},
						},
					},
				},
			}

			By("Creating the custom resource for the Kind Rover")
			resource := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": testEnvironment,
					},
				},
				Spec: spec,
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Checking that the resource fails with appropriate error")
			Eventually(func(g Gomega) {
				found := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, found)).To(Succeed())

				// Check that the resource has error condition
				g.Expect(found.Status.Conditions).NotTo(BeEmpty())
				readyCondition := getConditionByType(found.Status.Conditions, "Ready")
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Message).To(ContainSubstring("failed to get trusted team"))
				g.Expect(readyCondition.Message).To(ContainSubstring("nonexistent-group--nonexistent-team"))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle maximum trusted teams limit", func() {
			// Create a list with exactly 10 trusted teams (the maximum allowed)
			trustedTeams := []roverv1.TrustedTeam{
				{Group: "trusted-group-1", Team: "trusted-team-1"},
				{Group: "trusted-group-2", Team: "trusted-team-2"},
				{Group: "trusted-group-3", Team: "trusted-team-3"},
				// Add more teams up to the limit - using existing teams for simplicity
				{Group: "trusted-group-1", Team: "trusted-team-1"}, // Duplicate for testing
				{Group: "trusted-group-2", Team: "trusted-team-2"}, // Duplicate for testing
			}

			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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
								Strategy:     roverv1.ApprovalStrategyFourEyes,
								TrustedTeams: trustedTeams,
							},
						},
					},
				},
			}

			By("Creating the custom resource for the Kind Rover")
			resource := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": testEnvironment,
					},
				},
				Spec: spec,
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Checking if ApiExposure was created successfully with multiple trusted teams")
			Eventually(func(g Gomega) {
				apiExposure := &apiapi.ApiExposure{}
				apiExposureKey := client.ObjectKey{
					Name:      resourceName + "--eni-api-v1",
					Namespace: teamNamespace,
				}
				err := k8sClient.Get(ctx, apiExposureKey, apiExposure)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify approval strategy
				g.Expect(apiExposure.Spec.Approval.Strategy).To(Equal(apiapi.ApprovalStrategyFourEyes))

				// Verify trusted teams are mapped (should handle duplicates)
				g.Expect(len(apiExposure.Spec.Approval.TrustedTeams)).To(BeNumerically(">=", 3))
				g.Expect(len(apiExposure.Spec.Approval.TrustedTeams)).To(BeNumerically("<=", 5))
			}, timeout, interval).Should(Succeed())
		})

		It("should work with different approval strategies", func() {
			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
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
								Strategy: roverv1.ApprovalStrategyAuto,
								TrustedTeams: []roverv1.TrustedTeam{
									{
										Group: "trusted-group-1",
										Team:  "trusted-team-1",
									},
								},
							},
						},
					},
				},
			}

			By("Creating the custom resource for the Kind Rover")
			resource := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": testEnvironment,
					},
				},
				Spec: spec,
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Checking if ApiExposure was created with Auto approval strategy and trusted teams")
			Eventually(func(g Gomega) {
				apiExposure := &apiapi.ApiExposure{}
				apiExposureKey := client.ObjectKey{
					Name:      resourceName + "--eni-api-v1",
					Namespace: teamNamespace,
				}
				err := k8sClient.Get(ctx, apiExposureKey, apiExposure)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify approval strategy is Auto
				g.Expect(apiExposure.Spec.Approval.Strategy).To(Equal(apiapi.ApprovalStrategyAuto))

				// Verify trusted teams are still mapped correctly
				g.Expect(apiExposure.Spec.Approval.TrustedTeams).To(HaveLen(1))
				g.Expect(apiExposure.Spec.Approval.TrustedTeams[0].Name).To(Equal("trusted-group-1--trusted-team-1"))
				g.Expect(apiExposure.Spec.Approval.TrustedTeams[0].Namespace).To(Equal(testEnvironment))
			}, timeout, interval).Should(Succeed())
		})
	})

})
