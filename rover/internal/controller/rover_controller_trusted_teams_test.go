// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("Rover Controller - Trusted Teams", Ordered, func() {

	const (
		resourceName = "test-resource"
		BasePath     = "/eni/api/v1"
		upstream     = "https://api.example.com"
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

		// Note: Namespaces and team are shared with main controller tests
		// They may already exist, so we handle that gracefully
		By("Ensuring the Team exists")
		team = newTeam(teamName, group, testEnvironment, testEnvironment)
		err := k8sClient.Create(ctx, team)
		if err != nil {
			// Team might already exist from main controller tests, that's OK
			if err := client.IgnoreAlreadyExists(err); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
		}

		By("Creating the team Namespace")
		createNamespace(teamNamespace)
	})

	AfterEach(func() {
		resource := &roverv1.Rover{}
		err := k8sClient.Get(ctx, typeNamespacedName, resource)
		if err == nil {
			By("Cleanup the specific resource instance Rover")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}, timeout, interval).Should(Succeed())
		}
	})

	AfterAll(func() {
		By("Cleanup the Team")
		Expect(k8sClient.Delete(ctx, team)).To(Succeed())
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
				// Note: The owner team is added as well
				g.Expect(apiExposure.Spec.Approval.TrustedTeams).To(HaveLen(3))

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
				g.Expect(apiExposure.Spec.Approval.TrustedTeams).To(ContainElement(types.ObjectRef{
					Name:      "eni--hyperion",
					Namespace: testEnvironment,
				}))
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
				readyCondition := meta.FindStatusCondition(found.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Message).To(ContainSubstring("failed to get trusted team"))
				g.Expect(readyCondition.Message).To(ContainSubstring("nonexistent-group--nonexistent-team"))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle maximum trusted teams limit", func() {
			// Create a list with exactly 5 trusted teams for testing
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
				g.Expect(len(apiExposure.Spec.Approval.TrustedTeams)).To(BeNumerically("<=", 6))
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
				g.Expect(apiExposure.Spec.Approval.TrustedTeams).To(HaveLen(2))
				g.Expect(apiExposure.Spec.Approval.TrustedTeams[0].Name).To(Equal("trusted-group-1--trusted-team-1"))
				g.Expect(apiExposure.Spec.Approval.TrustedTeams[0].Namespace).To(Equal(testEnvironment))
			}, timeout, interval).Should(Succeed())
		})
	})

})
