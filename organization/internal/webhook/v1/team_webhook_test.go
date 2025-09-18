// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	testMember = []organizationv1.Member{{Email: "test@example.com", Name: "member"}}
)

var _ = Describe("Team Webhook", func() {
	var (
		teamObj   *organizationv1.Team
		validator TeamCustomValidator
	)
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

	BeforeEach(func() {
		By("Creating the Zone")
		freshZone := zone.DeepCopy()
		freshZone.ResourceVersion = ""
		err := k8sClient.Create(ctx, freshZone)
		Expect(err).NotTo(HaveOccurred())

		freshZone.Status = zoneStatus
		err = k8sClient.Status().Update(ctx, freshZone)
		Expect(err).NotTo(HaveOccurred())

		zone = freshZone

		teamObj = &organizationv1.Team{}
		validator = TeamCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(teamObj).NotTo(BeNil(), "Expected teamObj to be initialized")
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, zone)).NotTo(HaveOccurred())
	})

	Context("When CreateOrUpdate a valid team", Ordered, func() {
		It("should return no error on valid settings", func() {
			By("Creating a team with name: spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateCreate(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
			warning, err = validator.ValidateDelete(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})
		It("should return same result as create", func() {
			By("Updating a team with name: spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateUpdate(ctx, teamObj, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
			warning, err = validator.ValidateDelete(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When CreateOrUpdate an invalid team", func() {
		It("should return an error", func() {
			By("Creating a Team with name completely different from spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "here-is-a--complete-mismatch",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateCreate(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("must be equal to 'spec.group--spec.name'"))
		})
		It("should return an error since env is missing", func() {
			By("Creating a Team with name completely different from spec.group--spec.name")
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			warning, err := validator.ValidateCreate(ctx, teamObj)
			Expect(warning).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("must contain an environment label"))
		})
	})

	Context("When inserting a wrong kind", func() {
		It("should return an error", func() {
			groupObj := &organizationv1.Group{}
			warning, err := validator.ValidateCreate(ctx, groupObj)
			Expect(warning).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to convert object to team object"))
		})
	})

	Context("When inserting an valid team against the k8s", Ordered, func() {
		var teamObj *organizationv1.Team
		BeforeAll(func() {
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group-test--team-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			err := k8sClient.Create(ctx, teamObj)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(
			func() {
				By("Deleting the team")
				err := k8sClient.Delete(ctx, teamObj)
				Expect(err).NotTo(HaveOccurred())
			})

		It("should set secret", func() {
			Eventually(func(g Gomega) {
				By("Checking the team secret to be set")
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(teamObj), teamObj)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(teamObj.Spec.Secret).NotTo(BeEmpty())
				g.Expect(strings.HasPrefix(teamObj.Spec.Secret, "$<")).To(BeTrueBecause("client secret does not end with $<"))
				g.Expect(strings.HasSuffix(teamObj.Spec.Secret, ">")).To(BeTrueBecause("client secret does not end with >"))
			}, timeout, interval).Should(Succeed())
		})
		It("should update the secret if empty", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(teamObj), teamObj)
			Expect(err).NotTo(HaveOccurred())
			By("Setting the secret to empty")
			teamObj.Spec.Secret = ""
			err = k8sClient.Update(ctx, teamObj)
			Eventually(func(g Gomega) {
				By("Checking the team secret to be set")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(teamObj), teamObj)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(teamObj.Spec.Secret).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
		It("should rotate the secret if rotate", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(teamObj), teamObj)
			Expect(err).NotTo(HaveOccurred())
			By("Setting the secret to rotate")
			teamObj.Spec.Secret = "rotate"
			err = k8sClient.Update(ctx, teamObj)
			Eventually(func(g Gomega) {
				By("Checking the team secret to be updated")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(teamObj), teamObj)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(teamObj.Spec.Secret).NotTo(BeEmpty())
				g.Expect(teamObj.Spec.Secret).NotTo(BeEquivalentTo("rotate"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("When inserting an invalid team against  k8s", func() {
		It("should return an error from the webhook", func() {
			teamObj = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "here-is-a--complete-mismatch",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: organizationv1.TeamSpec{
					Group:   "group-test",
					Name:    "team-test",
					Email:   "test@example.com",
					Members: testMember,
				},
			}
			err := k8sClient.Create(ctx, teamObj)
			Expect(errors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Invalid value: \"here-is-a--complete-mismatch\": must be equal to 'spec.group--spec.name'"))
		})
	})

})
