// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/organization/internal/secret"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
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
			Name:     name,
			Group:    group,
			Email:    testMail,
			Members:  members,
			Category: organizationv1.TeamCategoryCustomer,
		},
		Status: organizationv1.TeamStatus{},
	}
}

var _ = Describe("Team Controller", Ordered, func() {

	var secretManagerMock *fake.MockSecretManager

	Context("Zone with TeamApis is available", Ordered, func() {
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
				Url:       "http://example.org",
				Issuer:    "http://example.org/issuer",
				LmsIssuer: "http://example.org/lms-issuer",
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

			By("Mocking Secret Manager")
			secretManagerMock = fake.NewMockSecretManager(GinkgoT())
			secretManagerMock.EXPECT().UpsertTeam(mock.Anything, mock.Anything, mock.Anything).Return(
				map[string]string{
					"teamToken": string(uuid.NewUUID()),
				}, nil)

			secretManagerMock.EXPECT().DeleteTeam(mock.Anything, mock.Anything, mock.Anything).Return(nil)

			secret.GetSecretManager = func() api.SecretManager {
				return secretManagerMock
			}
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, zone)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("Create a single team, happy path", Ordered, func() {
			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group
			const teamName = "team-alpha"
			const groupName = "group-alpha"
			const expectedTeamNamespaceName = testEnvironment + "--" + groupName + "--" + teamName

			BeforeAll(func() {
				By("Initializing the Team & Group")
				team = NewTeam(teamName, groupName, []organizationv1.Member{{Email: testMail, Name: "member"}})
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

					By("Checking if the Team namespace is being terminated")
					ns := newNamespaceObj(team.Status.Namespace)
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), ns)
					// EnvTest does not support namespace deletion. See: https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(isNamespaceTerminating(ns.Status)).To(BeTrue())

					By("Checking gateway consumer deletion")
					err = k8sClient.Get(ctx, team.Status.GatewayConsumerRef.K8s(), &gatewayv1.Consumer{})
					g.Expect(errors.IsNotFound(err)).To(BeTrue())

					By("Checking identity client deletion")
					err = k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), &identityv1.Client{})
					g.Expect(errors.IsNotFound(err)).To(BeTrue())

					By("Checking notification channel deletion")
					err = k8sClient.Get(ctx, team.Status.NotificationChannelRef.K8s(), &notificationv1.NotificationChannel{})
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}, timeout, interval).Should(Succeed())
			})

			It("should be ready and all resources created", func() {
				err = k8sClient.Create(ctx, group)
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Create(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)
				Expect(err).NotTo(HaveOccurred())

				By("Checking if the Team is ready")
				Eventually(func(g Gomega) {
					By("Getting the latest version of team object")
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
					g.Expect(err).NotTo(HaveOccurred())
					ExpectObjConditionToBeReady(g, team)

					By("Checking the team namespace in status")
					g.Expect(team.Status.Namespace).To(Equal(expectedTeamNamespaceName))

					By("Checking the namespace object")
					ns := newNamespaceObj(team.Status.Namespace)
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), ns)).NotTo(HaveOccurred())
					g.Expect(ns.Name).To(BeEquivalentTo(expectedTeamNamespaceName))

					By("Checking the namespace object labels")
					g.Expect(ns.GetLabels()).To(BeEquivalentTo(map[string]string{
						config.EnvironmentLabelKey:    testEnvironment,
						"kubernetes.io/metadata.name": expectedTeamNamespaceName,
					}))

					By("Checking the team identity client ref")
					g.Expect(team.Status.IdentityClientRef.String()).To(Equal(expectedTeamNamespaceName + "/" + groupName + "--" + teamName + "--team-user"))
					By("Checking the team token")
					g.Expect(team.Status.TeamToken).ToNot(BeEmpty()) //should have value from mock

					By("Checking the team identity client object")
					var identityClient = &identityv1.Client{}
					g.Expect(k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)).NotTo(HaveOccurred())

					By("Checking the team identity client object spec")
					identityClient.Spec.ClientSecret = "<obfuscated>"
					g.Expect(identityClient.Spec).
						To(BeEquivalentTo(identityv1.ClientSpec{
							Realm: &types.ObjectRef{
								Name:      "team-api-identity-realm",
								Namespace: "default",
								UID:       "",
							},
							ClientId:     groupName + "--" + teamName + "--team-user",
							ClientSecret: "<obfuscated>",
						}))

					By("Checking the team identity client object labels")
					g.Expect(identityClient.GetLabels()).To(BeEquivalentTo(map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					}))

					By("Checking the team gateway consumer ref")
					g.Expect(team.Status.GatewayConsumerRef.String()).To(Equal(expectedTeamNamespaceName + "/" + groupName + "--" + teamName + "--team-user"))

					By("Checking the team gateway consumer object")
					var gatewayConsumer = &gatewayv1.Consumer{}
					g.Expect(k8sClient.Get(ctx, team.Status.GatewayConsumerRef.K8s(), gatewayConsumer)).NotTo(HaveOccurred())

					By("Checking the team gateway consumer object spec")
					g.Expect(gatewayConsumer.Spec).To(BeEquivalentTo(gatewayv1.ConsumerSpec{
						Realm: types.ObjectRef{
							Name:      "team-api-gateway-realm",
							Namespace: "default",
							UID:       "",
						},
						Name: groupName + "--" + teamName + "--team-user",
					}))

					By("Checking the team gateway consumer object labels")
					g.Expect(gatewayConsumer.GetLabels()).To(BeEquivalentTo(map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					}))

					By("Checking the notification channel ref")
					g.Expect(team.Status.NotificationChannelRef).NotTo(BeNil())
					g.Expect(team.Status.NotificationChannelRef.String()).To(Equal(expectedTeamNamespaceName + "/" + groupName + "--" + teamName))

					By("Checking the notification channel object")
					var notificationChannel = &notificationv1.NotificationChannel{}
					g.Expect(k8sClient.Get(ctx, team.Status.NotificationChannelRef.K8s(), notificationChannel)).NotTo(HaveOccurred())

					By("Checking the notification channel email config")
					g.Expect(notificationChannel.Spec.Email).NotTo(BeNil())
					g.Expect(notificationChannel.Spec.Email.Recipients).To(ContainElement(testMail))

					By("Checking the notification channel labels")
					g.Expect(notificationChannel.GetLabels()).To(BeEquivalentTo(map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					}))

					By("Checking onboarding notification was created")
					g.Expect(team.Status.NotificationsRef["onboarded"]).NotTo(BeNil())
					var onboardingNotification = &notificationv1.Notification{}
					g.Expect(k8sClient.Get(ctx, team.Status.NotificationsRef["onboarded"].K8s(), onboardingNotification)).NotTo(HaveOccurred())
					g.Expect(onboardingNotification.Spec.Purpose).To(Equal("onboarded"))

					By("Checking token rotation notification was created")
					g.Expect(team.Status.NotificationsRef["token-rotated"]).NotTo(BeNil())
					var tokenNotification = &notificationv1.Notification{}
					g.Expect(k8sClient.Get(ctx, team.Status.NotificationsRef["token-rotated"].K8s(), tokenNotification)).NotTo(HaveOccurred())
					g.Expect(tokenNotification.Spec.Purpose).To(Equal("token-rotated"))

				}, timeout, interval).Should(Succeed())

				By("Updating team members to trigger member change notification")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
				Expect(err).NotTo(HaveOccurred())

				originalMemberChangeRef := team.Status.NotificationsRef["team-members-changed"]
				team.Spec.Members = append(team.Spec.Members, organizationv1.Member{
					Name:  "new-member",
					Email: "newmember@example.com",
				})
				err = k8sClient.Update(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					By("Getting the updated team")
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
					g.Expect(err).NotTo(HaveOccurred())

					By("Checking member change notification was created")
					g.Expect(team.Status.NotificationsRef["team-members-changed"]).NotTo(BeNil())
					g.Expect(team.Status.NotificationsRef["team-members-changed"]).NotTo(Equal(originalMemberChangeRef))

					var memberChangeNotification = &notificationv1.Notification{}
					g.Expect(k8sClient.Get(ctx, team.Status.NotificationsRef["team-members-changed"].K8s(), memberChangeNotification)).NotTo(HaveOccurred())
					g.Expect(memberChangeNotification.Spec.Purpose).To(Equal("team-members-changed"))
				}, timeout, interval).Should(Succeed())
			})
		})
		Context("Deleting teams with missing refs", func() {
			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group
			const teamName = "team-zeta"
			const groupName = "group-zeta"

			BeforeEach(func() {
				By("Initializing the Team & Group")
				team = NewTeam(teamName, groupName, []organizationv1.Member{{Email: testMail, Name: "member"}})
				group = NewGroupForTeam(team)
			})

			AfterEach(func() {
				By("Tearing down the Groups")
				err = k8sClient.Delete(ctx, group)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should be deleted handled without errors", func() {
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
					g.Expect(team.Status.TeamToken).NotTo(BeEmpty())
				}, timeout, interval).Should(Succeed())

				By("housekeeping the referred idp-c object in advance to keep env clean")
				var identityClient = &identityv1.Client{}
				Expect(team.Status.IdentityClientRef).NotTo(BeNil())
				err = k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, identityClient)).NotTo(HaveOccurred())

				By("housekeeping the referred gw-c object in advance to keep env clean")
				var gatewayConsumer = &gatewayv1.Consumer{}
				err = k8sClient.Get(ctx, team.Status.GatewayConsumerRef.K8s(), gatewayConsumer)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, gatewayConsumer)).NotTo(HaveOccurred())

				By("Modifying the team status to remove refs")
				team.Status.IdentityClientRef = nil
				team.Status.GatewayConsumerRef = nil
				err = k8sClient.Status().Update(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				By("By deleting the team which points to non-existing idp-c and gw-c")
				err := k8sClient.Delete(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					By("Checking if the Team namespace is being terminated")
					ns := newNamespaceObj(team.Status.Namespace)
					err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), ns)
					// EnvTest does not support namespace deletion. See: https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(isNamespaceTerminating(ns.Status)).To(BeTrue())
				}, timeout, interval).Should(Succeed())

			})
		})
		Context("Deleting teams with refs pointing to objects that doesn't exist anymore", func() {
			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group
			const teamName = "team-epsilon"
			const groupName = "group-epsilon"

			BeforeEach(func() {
				By("Initializing the Team & Group")
				team = NewTeam(teamName, groupName, []organizationv1.Member{{Email: testMail, Name: "member"}})
				group = NewGroupForTeam(team)
			})

			AfterEach(func() {
				By("Tearing down the Groups")
				err = k8sClient.Delete(ctx, group)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should be deleted handled without errors", func() {
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
				}, timeout, interval).Should(Succeed())

				By("delete idp-c")
				var identityClient = &identityv1.Client{}
				Expect(team.Status.IdentityClientRef).NotTo(BeNil())
				err = k8sClient.Get(ctx, team.Status.IdentityClientRef.K8s(), identityClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, identityClient)).NotTo(HaveOccurred())

				By("delete gw-c")
				var gatewayConsumer = &gatewayv1.Consumer{}
				err = k8sClient.Get(ctx, team.Status.GatewayConsumerRef.K8s(), gatewayConsumer)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, gatewayConsumer)).NotTo(HaveOccurred())

				By("By deleting the team which points to non-existing idp-c and gw-c")
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Delete(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					By("Checking if the Team namespace is being terminated")
					ns := newNamespaceObj(team.Status.Namespace)
					err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), ns)
					// EnvTest does not support namespace deletion. See: https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(isNamespaceTerminating(ns.Status)).To(BeTrue())
				}, timeout, interval).Should(Succeed())

			})
		})
		Context("Reject a invalid teams", func() {
			AfterEach(func() {
				By("Tearing down the Team")
				err := k8sClient.DeleteAllOf(ctx, &organizationv1.Team{}, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.DeleteAllOf(ctx, &organizationv1.Group{}, client.InNamespace(testEnvironment))
				Expect(err).NotTo(HaveOccurred())
			})

			It("should rejected invalid spec.name", func() {
				By("Creating an invalid team")
				invalidTeam := NewTeam("invalid--name-with-double-dashes", "group", []organizationv1.Member{{Email: testMail, Name: "member"}})
				err := k8sClient.Create(ctx, invalidTeam)

				By("Receiving invalid error")
				Expect(errors.IsInvalid(err)).To(BeTrue())

				By("Receiving not finding the resource")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(invalidTeam), invalidTeam)
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
			It("should rejected invalid spec.group", func() {
				By("Creating an invalid team")
				invalidTeam := NewTeam("valid-team", "invalid--group-with-double-dashes", []organizationv1.Member{{Email: testMail, Name: "member"}})
				err := k8sClient.Create(ctx, invalidTeam)

				By("Receiving invalid error")
				Expect(errors.IsInvalid(err)).To(BeTrue())

				By("Receiving not finding the resource")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(invalidTeam), invalidTeam)
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("Group is missing", func() {
			var err error
			var team *organizationv1.Team
			const teamName = "team-beta"
			const nameOfMissingGroup = "group-beta-missing"

			BeforeEach(func() {
				By("Initializing the Team")
				team = NewTeam(teamName, nameOfMissingGroup, []organizationv1.Member{{Email: testMail, Name: "member"}})
			})

			AfterEach(func() {
				By("Tearing down the Team")
				Expect(k8sClient.Delete(ctx, team)).NotTo(HaveOccurred())
			})

			It("should not found the group", func() {
				err = k8sClient.Create(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				By("Checking if the Team becomes blocked")
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)).NotTo(HaveOccurred())

					By("Checking the conditions")
					g.Expect(team.Status.Conditions).To(HaveLen(2))
					processingCondition := meta.FindStatusCondition(team.Status.Conditions, condition.ConditionTypeProcessing)
					g.Expect(processingCondition).NotTo(BeNil())
					g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
					Expect(processingCondition.Message).To(ContainSubstring("Group not found"))
				}, timeout, interval).Should(Succeed())
			})
		})
		Context("Environment label is missing", func() {
			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group
			teamName := "team-gamma"
			groupName := "group-gamma"

			BeforeEach(func() {
				By("Initializing the Team")
				team = NewTeam(teamName, groupName, []organizationv1.Member{{Email: testMail, Name: "member"}})
				group = NewGroupForTeam(team)
			})

			AfterEach(func() {
				By("Tearing down the Team")
				Expect(k8sClient.Delete(ctx, team)).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, group)).NotTo(HaveOccurred())
			})

			It("should not have empty labels", func() {
				Expect(k8sClient.Create(ctx, group)).ToNot(HaveOccurred())

				team.SetLabels(make(map[string]string))
				err = k8sClient.Create(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				By("Checking if the Team becomes blocked")
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)).NotTo(HaveOccurred())

					By("Checking the conditions")
					g.Expect(team.Status.Conditions).To(HaveLen(2))
					failedCondition := meta.FindStatusCondition(team.Status.Conditions, condition.ConditionTypeReady)
					g.Expect(failedCondition).NotTo(BeNil())
					g.Expect(failedCondition.Status).To(Equal(metav1.ConditionUnknown)) // common.Reconcile triggers before any handler logic to check envs. See common library
				}, timeout, interval).Should(Succeed())
			})
		})
	})
	Context("Zone with TeamApis is unavailable", Ordered, func() {
		zone := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testEnvironment,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: adminv1.ZoneSpec{
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}

		BeforeAll(func() {
			By("Creating the Zone")
			err := k8sClient.Create(ctx, zone)
			Expect(err).NotTo(HaveOccurred())
			By("Checking if the zone realm refs are nil")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.TeamApiGatewayRealm).To(BeNil())
			Expect(zone.Status.TeamApiIdentityRealm).To(BeNil())
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, zone)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("Create a single team remains in status processing", Ordered, func() {
			var err error
			var team *organizationv1.Team
			var group *organizationv1.Group
			const teamName = "team-alpha"
			const groupName = "group-alpha"
			const expectedTeamNamespaceName = testEnvironment + "--" + groupName + "--" + teamName

			BeforeAll(func() {
				By("Initializing the Team & Group")
				team = NewTeam(teamName, groupName, []organizationv1.Member{{Email: testMail, Name: "member"}})
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
					By("Checking if the Team namespace is being terminated")
					ns := newNamespaceObj(team.Status.Namespace)
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ns), ns)
					// EnvTest does not support namespace deletion. See: https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(isNamespaceTerminating(ns.Status)).To(BeTrue())
				}, timeout, interval).Should(Succeed())
			})

			It("should be blocked", func() {
				err = k8sClient.Create(ctx, group)
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Create(ctx, team)
				Expect(err).NotTo(HaveOccurred())

				By("Checking if ErrorOccurred in team processing")
				Eventually(func(g Gomega) {
					By("Getting the latest version of team object")
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), team)
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(team.GetConditions()).To(HaveLen(2))
					readyCondition := meta.FindStatusCondition(team.GetConditions(), condition.ConditionTypeReady)
					g.Expect(readyCondition).NotTo(BeNil())
					g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(readyCondition.Reason).To(Equal("ErrorOccurred"))
					g.Expect(readyCondition.Message).To(ContainSubstring("found no zone with team apis"))

					By("Checking the team namespace in status")
					g.Expect(team.Status.Namespace).To(Equal(expectedTeamNamespaceName))
				}, timeout, interval).Should(Succeed())
			})
		})
	})

})

func isNamespaceTerminating(namespaceStatus corev1.NamespaceStatus) bool {
	return namespaceStatus.Phase == corev1.NamespaceTerminating
}

func newNamespaceObj(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
