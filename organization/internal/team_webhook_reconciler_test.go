// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"k8s.io/apimachinery/pkg/api/errors"
	_ "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewGroupForTeam(teamObj *organizationv1.Team) *organizationv1.Group {
	return &organizationv1.Group{
		ObjectMeta: metav1.ObjectMeta{
			Name:      teamObj.Spec.Group,
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: organizationv1.GroupSpec{
			DisplayName: teamObj.Spec.Group,
			Description: "Group example",
		},
		Status: organizationv1.GroupStatus{},
	}
}

func NewTeam(name, group string, members []organizationv1.Member) *organizationv1.Team {
	return &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      group + "--" + name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: organizationv1.TeamSpec{
			Name:    name,
			Group:   group,
			Email:   "example@example.com",
			Members: members,
		},
		Status: organizationv1.TeamStatus{},
	}
}

var _ = Describe("Team Reconciler, Group Reconciler and Team Webhook", Ordered, func() {
	Context("With Zone", func() {
		zone := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testEnvironment,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: adminv1.ZoneSpec{
				TeamApis: &adminv1.TeamApiConfig{Apis: []adminv1.ApiConfig{{
					Name: "team-api-1",
					Path: "/teamAPI",
					Url:  "http://example.org",
				}}},
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}

		zoneStatus := adminv1.ZoneStatus{
			TeamApiIdentityRealm: &types.ObjectRef{
				Name:      "team-api-identity-realm",
				Namespace: testNamespace,
			},
			TeamApiGatewayRealm: &types.ObjectRef{
				Name:      "team-api-gateway-realm",
				Namespace: testNamespace,
			},
		}

		BeforeAll(func() {
			By("Creating the Zone")
			err := k8sClient.Create(ctx, zone)
			Expect(err).NotTo(HaveOccurred())
			zone.Status = zoneStatus
			err = k8sClient.Status().Update(ctx, zone)
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the zone is status is updated")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.TeamApiGatewayRealm).NotTo(BeNil())
			Expect(zone.Status.TeamApiIdentityRealm).NotTo(BeNil())
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, zone)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("Create a single team. Happy path", Ordered, func() {

			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group
			const teamName = "team-alpha"
			const groupName = "group-alpha"
			const expectedTeamNamespaceName = testEnvironment + "--" + groupName + "--" + teamName

			BeforeAll(func() {
				By("Initializing the Team & Group")
				team = NewTeam(teamName, groupName, []organizationv1.Member{{Email: "example@example.com", Name: "member"}})
				group = NewGroupForTeam(team)
			})

			AfterAll(func() {
				By("Gathering references")
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
				Expect(err).NotTo(HaveOccurred())

				By("Tearing down the Teams & Groups")
				err = k8sClient.Delete(ctx, team)
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Delete(ctx, group)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					By("Checking if the identity client has been deleted")
					err = k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), &identityv1.Client{})
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}, timeout, interval).Should(Succeed())
			})

			It("should be ready and all resources created", func() {

				err = k8sClient.Create(ctx, group)
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Create(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				By("Checking if the Team is ready")
				Eventually(func(g Gomega) {
					By("Getting the latest version of team object")
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
					g.Expect(err).NotTo(HaveOccurred())
					ExpectObjConditionToBeReady(g, team)

					By("Checking the Reconciler changes in Status")
					By("Checking the team namespace in status")
					g.Expect(team.Status.Namespace).To(Equal(expectedTeamNamespaceName))

					By("Checking the team gateway consumer ref")
					g.Expect(team.Status.GatewayConsumerRef.String()).To(Equal(expectedTeamNamespaceName + "/" + groupName + "--" + teamName + "--team-user"))

					By("Checking the team identity client ref")
					g.Expect(team.Status.IdentityClientRef.String()).To(Equal(expectedTeamNamespaceName + "/" + groupName + "--" + teamName + "--team-user"))

					By("Checking the Webhook changes")
					By("Checking the set secret")
					g.Expect(team.Spec.Secret).NotTo(BeEmpty())

					By("Checking the team identity client secret to be the same")
					identityClient := &identityv1.Client{}
					g.Expect(k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)).NotTo(HaveOccurred())
					g.Expect(identityClient.Spec.ClientSecret).To(Equal(team.Spec.Secret))
				}, timeout, interval).Should(Succeed())
			})
			It("should be able to rotate the secret", func() {

				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
				Expect(err).NotTo(HaveOccurred())

				var previousToken, latestToken organizationv1.TeamToken
				// previousTokenRef := team.Status.TeamToken
				previousToken = getDecodedToken(team.Status.TeamToken)
				team.Spec.Secret = "rotate"
				By("Updating the Team.Spec to rotate with delay to have a different timestamp")
				time.Sleep(1 * time.Second)
				Expect(k8sClient.Update(ctx, team)).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					time.Sleep(1 * time.Second)
					By("Getting the latest version of team object")
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
					Expect(err).NotTo(HaveOccurred())
					ExpectObjConditionToBeReady(g, team)

					By("Checking the Webhook & Reconciler changes on the secret")
					latestToken = getDecodedToken(team.Status.TeamToken)
					g.Expect(team.Spec.Secret).NotTo(BeEmpty())
					g.Expect(previousToken.ClientSecret).NotTo(Equal(latestToken.ClientSecret))
					g.Expect(previousToken.ClientId).To(Equal(latestToken.ClientId))
					g.Expect(previousToken.Environment).To(Equal(latestToken.Environment))
					g.Expect(previousToken.GeneratedAt).To(BeNumerically("<", latestToken.GeneratedAt))

					By("Checking the team identity client secret to be the same")
					identityClient := &identityv1.Client{}
					g.Expect(k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)).NotTo(HaveOccurred())
					g.Expect(identityClient.Spec.ClientSecret).To(Equal(team.Spec.Secret))

					// By("Checking the previous secret has been deleted")
					// value, _ := secret.GetSecretManager().Get(ctx, previousTokenRef)
					// g.Expect(value).To(BeEmpty())
				}, timeout, interval).Should(Succeed())
			})

		})
		Context("Create a single team. Unhappy path", Ordered, func() {
			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group

			AfterEach(func() {
				By("Tearing down the Teams & Groups")
				err = k8sClient.DeleteAllOf(ctx, team)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())
				}
				err = k8sClient.DeleteAllOf(ctx, group)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should reject invalid names", func() {
				team = NewTeam("team", "group", []organizationv1.Member{{Email: "example@example.com", Name: "member"}})
				group = NewGroupForTeam(team)
				By("Creating the Group")
				err = k8sClient.Create(ctx, group)
				Expect(err).NotTo(HaveOccurred())

				By("changing the name to be invalid and dont match the expected pattern <group>--<team>")
				team.Spec.Name = "mismatch-in-name"
				err = k8sClient.Create(ctx, team)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("admission webhook \"vteam-v1.kb.io\" denied the request: Team.organization.cp.ei.telekom.de \"group--team\" is invalid: metadata.name: Invalid value: \"group--team\": must be equal to 'spec.group--spec.name'"))
			})
		})
	})

})

func getDecodedToken(secretId string) organizationv1.TeamToken {
	By("Getting the token from mocked secret manager")
	tokenEncoded, err := secret.GetSecretManager().Get(ctx, secretId)
	Expect(err).NotTo(HaveOccurred())
	tokenDecoded, err := organizationv1.DecodeTeamToken(tokenEncoded)
	Expect(err).NotTo(HaveOccurred())
	return tokenDecoded
}
