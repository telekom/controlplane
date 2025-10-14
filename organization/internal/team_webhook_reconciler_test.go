// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	"k8s.io/apimachinery/pkg/api/errors"
	_ "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var onboardingOptions = &api.OnboardingOptions{
	SecretValues: make(map[string]any),
}

var memoryTestStorage = make(map[string]string)

var secretManagerMock *fake.MockSecretManager

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
			Name:     name,
			Group:    group,
			Email:    "mail@example.com",
			Members:  members,
			Category: organizationv1.TeamCategoryCustomer,
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
			Links: adminv1.Links{
				Url:       "https://example.org",
				Issuer:    "https://example.org/issuer",
				LmsIssuer: "https://example.org/lms-issuer",
			},
		}

		BeforeAll(func() {

			secretManagerMock = fake.NewMockSecretManager(GinkgoT())
			secret.GetSecretManager = func() api.SecretManager {
				return secretManagerMock
			}

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
				team = NewTeam(teamName, groupName, []organizationv1.Member{{Email: "mail@example.com", Name: "member"}})
				group = NewGroupForTeam(team)
			})

			AfterAll(func() {
				By("Gathering references")
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
				Expect(err).NotTo(HaveOccurred())

				By("Tearing down the Teams & Groups")
				secretManagerMock.EXPECT().
					DeleteTeam(mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
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
				secretManagerMock.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(runAndReturnForUpsertTeam())
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
				previousToken = getDecodedToken(team.Status.TeamToken)
				previousTokenRotateRef := team.Status.NotificationsRef["token-rotated"]
				team.Spec.Secret = "rotate"

				By("Updating the Team.Spec to rotate with delay to have a different timestamp")
				time.Sleep(1 * time.Second)
				secretManagerMock.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(runAndReturnForUpsertTeam())

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

					By("Checking new token rotation notification was created")
					g.Expect(team.Status.NotificationsRef["token-rotated"]).NotTo(BeNil())
					g.Expect(team.Status.NotificationsRef["token-rotated"].Name).NotTo(Equal(previousTokenRotateRef.Name))
					var tokenNotification = &notificationv1.Notification{}
					g.Expect(k8sClient.Get(ctx, team.Status.NotificationsRef["token-rotated"].K8s(), tokenNotification)).NotTo(HaveOccurred())
					g.Expect(tokenNotification.Spec.Purpose).To(Equal("token-rotated"))

				}, timeout, interval).Should(Succeed())
			})
			It("should watch identity clients and update team token with reconciler", func() {
				By("Getting the latest version of team and identity client")
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)).ToNot(HaveOccurred())
				previousTokenReference := team.Status.TeamToken
				identityClient := &identityv1.Client{}
				Expect(k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)).ToNot(HaveOccurred())
				By("Waiting 1 seconds to have an actual difference in the generation time stamps")
				time.Sleep(1 * time.Second)
				Eventually(func(g Gomega) {
					By("Updating the client secret in the identity client")
					identityClient.Spec.ClientSecret = "<obfuscated-new-secret>"
					Expect(k8sClient.Update(ctx, identityClient)).ToNot(HaveOccurred())
					By("Waiting 2 seconds to give time to process the update")
					time.Sleep(2 * time.Second)

					By("Getting the latest version of team object")
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)).ToNot(HaveOccurred())
					ExpectObjConditionToBeReady(g, team)

					By("Comparing that the team token has not changed")
					latestTokenReference := team.Status.TeamToken
					compareToken(latestTokenReference, previousTokenReference, "==", "==")
				}, timeout, interval).Should(Succeed())

			})
			It("should be updated and return sub-resources to desired state", func() {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)).ToNot(HaveOccurred())

				previousTokenReference := team.Status.TeamToken
				By("Making undesired changes to id-c")
				var identityClient = &identityv1.Client{}
				Expect(k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)).ToNot(HaveOccurred())
				identityClient.Spec = identityv1.ClientSpec{
					Realm:        types.ObjectRefFromObject(identityClient),
					ClientId:     "invalid-id",
					ClientSecret: "invalid-secret",
				}
				Expect(k8sClient.Update(ctx, identityClient)).ToNot(HaveOccurred())

				By("Changing the Team Members")
				team.Spec.Members = append(team.Spec.Members, organizationv1.Member{
					Name:  "member2",
					Email: "mail@example.com",
				})
				Expect(k8sClient.Update(ctx, team)).ToNot(HaveOccurred())
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)).ToNot(HaveOccurred())
					ExpectObjConditionToBeReady(g, team)
					g.Expect(team.Status.IdentityClientRef).NotTo(BeNil())
					By("Checking the team changes have accorded")
					g.Expect(team.Spec.Name).To(Equal(teamName))
					g.Expect(team.Spec.Group).To(Equal(groupName))
					g.Expect(team.Spec.Email).To(Equal("mail@example.com"))
					g.Expect(team.Spec.Members).To(ConsistOf(
						organizationv1.Member{Email: "mail@example.com", Name: "member"},
						organizationv1.Member{Email: "mail@example.com", Name: "member2"},
					))
					g.Expect(team.Spec.Category).To(Equal(organizationv1.TeamCategoryCustomer))
					g.Expect(team.Spec.Secret).To(HavePrefix("$<testgroup-alpha--team-alphasecret-"))
					g.Expect(team.Spec.Secret).To(HaveSuffix(">"))
					By("Checking the team identity client is back to desired state")
					g.Expect(k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)).ToNot(HaveOccurred())
					identityClient.Spec.ClientSecret = "<obfuscated>"
					g.Expect(identityClient.Spec).To(BeEquivalentTo(identityv1.ClientSpec{
						Realm: &types.ObjectRef{
							Name:      "team-api-identity-realm",
							Namespace: "default",
							UID:       "",
						},
						ClientId:     groupName + "--" + teamName + "--team-user",
						ClientSecret: "<obfuscated>",
					}))

					By("Comparing that the team token has not changed")
					latestTokenReference := team.Status.TeamToken
					compareToken(latestTokenReference, previousTokenReference, "==", "==")
				}, timeout, interval).Should(Succeed())

			})
		})
		Context("Create a single team. Unhappy path", Ordered, func() {
			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group

			AfterEach(func() {
				By("Tearing down the Teams & Groups")
				secretManagerMock.EXPECT().
					DeleteTeam(mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
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
				team = NewTeam("team", "group", []organizationv1.Member{{Email: "mail@example.com", Name: "member"}})
				group = NewGroupForTeam(team)
				By("Creating the Group")
				secretManagerMock.EXPECT().
					UpsertTeam(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(runAndReturnForUpsertTeam())

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

func runAndReturnForUpsertTeam() func(ctx2 context.Context, s string, s2 string, option ...api.OnboardingOption) (map[string]string, error) {
	return func(ctx2 context.Context, s string, s2 string, option ...api.OnboardingOption) (map[string]string, error) {
		for i := range option {
			option[i](onboardingOptions)
		}

		tokenValue := fmt.Sprintf("%s", onboardingOptions.SecretValues["teamToken"])
		tokenHash := sha256.Sum256([]byte(tokenValue))
		tokenId := s + s2 + "token-" + hex.EncodeToString(tokenHash[:8])
		memoryTestStorage[tokenId] = tokenValue

		secretValue := fmt.Sprintf("%s", onboardingOptions.SecretValues["clientSecret"])
		secretHash := sha256.Sum256([]byte(secretValue))
		secretId := s + s2 + "secret-" + hex.EncodeToString(secretHash[:8])
		memoryTestStorage[secretId] = secretValue

		return map[string]string{
			"clientSecret": secretId,
			"teamToken":    tokenId,
		}, nil
	}
}

func compareToken(stringTokenA, stringTokenB, secretComparator, timestampComparator string) {
	tokenADecoded := getDecodedToken(stringTokenA)
	tokenBDecoded := getDecodedToken(stringTokenB)

	By("Comparing the generation time stamps, by timestampComparator '" + timestampComparator + "'")
	Expect(tokenADecoded.GeneratedAt).To(BeNumerically(timestampComparator, tokenBDecoded.GeneratedAt))

	By("Comparing the client secret, by secretComparator '" + secretComparator + "'")
	if secretComparator == "==" {
		Expect(tokenADecoded.ClientSecret).To(BeEquivalentTo(tokenBDecoded.ClientSecret))
	} else {
		Expect(tokenADecoded.ClientSecret).ToNot(BeEquivalentTo(tokenBDecoded.ClientSecret))
	}
}

func getDecodedToken(secretId string) organizationv1.TeamToken {
	By("Getting the token from mocked secret manager")

	secretManagerMock.EXPECT().Get(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, secretId string) (string, error) {
			secretId, _ = api.FromRef(secretId)
			return memoryTestStorage[secretId], nil
		})

	tokenEncoded, err := secret.GetSecretManager().Get(ctx, secretId)
	Expect(err).NotTo(HaveOccurred())
	tokenDecoded, err := organizationv1.DecodeTeamToken(tokenEncoded)
	Expect(err).NotTo(HaveOccurred())
	return tokenDecoded
}
