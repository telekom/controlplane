// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createTeam(teamName, groupName, env string) types.ObjectRef {
	return types.ObjectRef{
		Name:      groupName + "--" + teamName,
		Namespace: env,
	}
}

// Helper function to create an application with a specific team and verify it exists
func setupAppWithTeam(appName, teamName string) *applicationv1.Application {
	app := CreateApplication(appName)
	app.Spec.Team = teamName
	err := k8sClient.Update(ctx, app)
	Expect(err).ToNot(HaveOccurred())

	// Verify application is created
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
		g.Expect(err).ToNot(HaveOccurred())
	}, timeout*2, interval).Should(Succeed())

	return app
}

// Helper function to verify approval strategy for a subscription
func verifyApprovalStrategy(subscription *apiapi.ApiSubscription, expectedStrategy approvalapi.ApprovalStrategy) {
	Eventually(func(g Gomega) {
		// Get the latest subscription status
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(subscription.Status.ApprovalRequest).ToNot(BeNil())

		// Get the approval request
		approvalReq := &approvalapi.ApprovalRequest{}
		err = k8sClient.Get(ctx, subscription.Status.ApprovalRequest.K8s(), approvalReq)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify the approval strategy
		g.Expect(approvalReq.Spec.Strategy).To(Equal(expectedStrategy))
	}, timeout*3, interval).Should(Succeed())
}

var _ = Describe("ApiSubscription Controller with Trusted Teams", Ordered, func() {
	var apiBasePath = "/apiexpctrl/trustedteams/v1"
	var zoneName = "apiexp-trustedteams"

	var apiExposure *apiv1.ApiExposure
	var api *apiv1.Api
	var zone *adminapi.Zone
	var team1, team2, team3 types.ObjectRef

	BeforeAll(func() {
		By("Creating the Zone")
		zone = CreateZone(zoneName)

		By("Creating the Gateway")
		realm := NewRealm(testEnvironment, zone.Name)
		err := k8sClient.Create(ctx, realm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating Teams")
		team1 = createTeam("team1", "group1", testEnvironment)
		team2 = createTeam("team2", "group2", testEnvironment)
		team3 = createTeam("team3", "group3", testEnvironment)

		By("Initializing the API")
		api = NewApi(apiBasePath)
		err = k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up and deleting all resources")
		Expect(k8sClient.Delete(ctx, api)).To(Succeed())
	})

	Context("ApiExposure with Trusted Teams Configuration", Ordered, func() {
		It("should create ApiExposure with trusted teams correctly", func() {
			By("Creating an ApiExposure with trusted teams")
			apiExposure = NewApiExposure(apiBasePath, zoneName)
			apiExposure.Spec.Approval = apiv1.Approval{
				Strategy: apiapi.ApprovalStrategyFourEyes,
				TrustedTeams: []types.ObjectRef{
					team1, team2,
				},
			}

			err := k8sClient.Create(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the ApiExposure was created with trusted teams")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Active).To(BeTrue())

				g.Expect(apiExposure.Spec.Approval.TrustedTeams).To(HaveLen(2))
				teamNames := []string{
					apiExposure.Spec.Approval.TrustedTeams[0].Name,
					apiExposure.Spec.Approval.TrustedTeams[1].Name,
				}
				g.Expect(teamNames).To(ConsistOf("group1--team1", "group2--team2"))
			}, timeout*3, interval).Should(Succeed())
		})

		It("should automatically approve subscription from trusted team", func() {
			By("Creating an application from trusted team1")
			setupAppWithTeam("trusted-team-app", team1.Name)

			By("Creating an ApiSubscription from trusted team")
			trustedSubscription := NewApiSubscription(apiBasePath, zoneName, "trusted-team-app")
			err := k8sClient.Create(ctx, trustedSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the subscription was auto-approved")
			verifyApprovalStrategy(trustedSubscription, approvalapi.ApprovalStrategyAuto)
		})

		It("should use configured approval strategy for non-trusted team", func() {
			By("Creating an application from non-trusted team")
			setupAppWithTeam("non-trusted-app", "outside-team")

			By("Creating an ApiSubscription from non-trusted team")
			nonTrustedSubscription := NewApiSubscription(apiBasePath, zoneName, "non-trusted-app")
			err := k8sClient.Create(ctx, nonTrustedSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the subscription uses the default approval strategy")
			verifyApprovalStrategy(nonTrustedSubscription, approvalapi.ApprovalStrategyFourEyes)
		})

		It("should handle case-insensitive team name matching", func() {
			By("Creating an application with mixed case team name")
			setupAppWithTeam("case-mixed-app", "GrOuP1--TeAm1")

			By("Creating an ApiSubscription from the mixed case team")
			caseMixedSub := NewApiSubscription(apiBasePath, zoneName, "case-mixed-app")
			err := k8sClient.Create(ctx, caseMixedSub)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the subscription was auto-approved despite case differences")
			verifyApprovalStrategy(caseMixedSub, approvalapi.ApprovalStrategyAuto)
		})

		It("should check subscriptions after trusted teams list has been updated", func() {
			// "group1--team1" and "group2--team2" are trusted teams
			By("Creating applications from different teams")
			setupAppWithTeam("team1-app", "group1--team1")
			setupAppWithTeam("team2-app", "group2--team2")
			setupAppWithTeam("team3-app", "group3--team3")

			By("Creating ApiSubscriptions from team1, team2 and team3")
			team1Sub := NewApiSubscription(apiBasePath, zoneName, "team1-app")
			err := k8sClient.Create(ctx, team1Sub)
			Expect(err).ToNot(HaveOccurred())

			team2Sub := NewApiSubscription(apiBasePath, zoneName, "team2-app")
			err = k8sClient.Create(ctx, team2Sub)
			Expect(err).ToNot(HaveOccurred())

			team3Sub := NewApiSubscription(apiBasePath, zoneName, "team3-app")
			err = k8sClient.Create(ctx, team3Sub)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying initial approval states")
			verifyApprovalStrategy(team1Sub, approvalapi.ApprovalStrategyAuto)
			verifyApprovalStrategy(team2Sub, approvalapi.ApprovalStrategyAuto)
			verifyApprovalStrategy(team3Sub, approvalapi.ApprovalStrategyFourEyes)

			By("Updating the trusted teams list to include team3 instead of team2")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			Expect(err).ToNot(HaveOccurred())
			apiExposure.Spec.Approval.TrustedTeams = []types.ObjectRef{
				team1,
				team3,
			}
			err = k8sClient.Update(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the ApiExposure was updated with new trusted teams")
			Eventually(func(g Gomega) {
				updatedExposure := &apiv1.ApiExposure{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), updatedExposure)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(updatedExposure.Spec.Approval.TrustedTeams).To(HaveLen(2))
				teamNames := []string{
					updatedExposure.Spec.Approval.TrustedTeams[0].Name,
					updatedExposure.Spec.Approval.TrustedTeams[1].Name,
				}
				g.Expect(teamNames).To(ConsistOf("group1--team1", "group3--team3"))
			}, timeout*3, interval).Should(Succeed())

			By("Checking that subscriptions were reprocessed after trusted teams update")
			// Team1 should remain auto-approved
			verifyApprovalStrategy(team1Sub, approvalapi.ApprovalStrategyAuto)

			// Team2 should now require manual approval
			verifyApprovalStrategy(team2Sub, approvalapi.ApprovalStrategyFourEyes)

			// Team3 should now be auto-approved
			verifyApprovalStrategy(team3Sub, approvalapi.ApprovalStrategyAuto)
		})

		It("should handle invalid namespace in trusted team reference", func() {
			By("Creating an ApiExposure with trusted team having invalid namespace")
			invalidNamespaceExposure := NewApiExposure(apiBasePath+"/badns", zoneName)
			invalidNamespaceExposure.Name = "badns-teams-exposure"

			invalidNamespaceExposure.Spec.Approval = apiv1.Approval{
				Strategy: apiapi.ApprovalStrategyFourEyes,
				TrustedTeams: []types.ObjectRef{
					{
						Name:      "group1--team1",
						Namespace: "non-existent-namespace", // Invalid namespace
					},
				},
			}

			err := k8sClient.Create(ctx, invalidNamespaceExposure)
			Expect(err).ToNot(HaveOccurred()) // Should be created despite invalid reference

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(invalidNamespaceExposure), invalidNamespaceExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(invalidNamespaceExposure.Spec.Approval.TrustedTeams).To(HaveLen(1))
				g.Expect(invalidNamespaceExposure.Spec.Approval.TrustedTeams[0].Namespace).To(Equal("non-existent-namespace"))
			}, timeout*3, interval).Should(Succeed())

			err = k8sClient.Delete(ctx, invalidNamespaceExposure)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
