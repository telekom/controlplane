// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

const (
	testMail = "test@example.com"
)

func NewGroup(name, displayName string) *organizationv1.Group {
	return &organizationv1.Group{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: organizationv1.GroupSpec{
			DisplayName: displayName,
			Description: "desc",
		},
	}
}

var _ = Describe("Group Controller", Ordered, func() {

	const groupNamePrefix = "group-controller-"
	var groupObj *organizationv1.Group
	var teamAlphaObj, teamBetaObj *organizationv1.Team

	Context("Creating Group without team", Ordered, func() {

		AfterEach(func() {
			By("Tearing down the Group")
			err := k8sClient.DeleteAllOf(ctx, &organizationv1.Group{}, client.InNamespace(testEnvironment))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be ready", func() {
			groupObj = NewGroup(groupNamePrefix+"alpha", groupNamePrefix+"alpha")
			err := k8sClient.Create(ctx, groupObj)
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the Group is Ready")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(groupObj), groupObj)
				g.Expect(err).NotTo(HaveOccurred())
				By("Checking the conditions")
				g.Expect(groupObj.Status.Conditions).To(HaveLen(2))
				readyCondition := meta.FindStatusCondition(groupObj.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))

			}, timeout, interval).Should(Succeed())
		})
		It("should be able to receive groupList", func() {
			groupObj = NewGroup(groupNamePrefix+"beta", groupNamePrefix+"beta")
			err := k8sClient.Create(ctx, groupObj)
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the Group is in list")
			Eventually(func(g Gomega) {
				groupList := &organizationv1.GroupList{}
				err := k8sClient.List(ctx, groupList)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(groupList.GetItems()).To(HaveLen(1))
			}, timeout, interval).Should(Succeed())
		})
		It("should rejected invalid metadata.name", func() {
			By("Creating an invalid Group")
			invalidGroup := NewGroup("invalid--name-with-double-dashes", "valid-displayname")
			err := k8sClient.Create(ctx, invalidGroup)

			By("Receiving invalid error")
			Expect(errors.IsInvalid(err)).To(BeTrue())

			By("Receiving not finding the resource")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(invalidGroup), invalidGroup)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
	Context("Creating Group with teams", Ordered, func() {
		groupName := "group-controller-beta"
		teamAlphaName := "team-alpha"
		teamBetaName := "team-beta"
		BeforeEach(func() {
			By("Initializing the groupObj")
			groupObj = NewGroup(groupName, groupName)
			teamAlphaObj = NewTeam(teamAlphaName, groupName, []organizationv1.Member{{Email: testMail, Name: "member"}})
			teamBetaObj = NewTeam(teamBetaName, groupName, []organizationv1.Member{{Email: testMail, Name: "member"}})
		})

		AfterEach(func() {
			By("Tearing down the Group")
			err := k8sClient.DeleteAllOf(ctx, &organizationv1.Group{}, client.InNamespace(testEnvironment))
			Expect(err).NotTo(HaveOccurred())

			By("Checking Teams of Group are deleted as well")
			Eventually(func(g Gomega) {
				g.Expect(
					errors.IsNotFound(
						k8sClient.Get(ctx, client.ObjectKeyFromObject(teamAlphaObj), teamAlphaObj))).
					To(BeTrue())
				g.Expect(
					errors.IsNotFound(
						k8sClient.Get(ctx, client.ObjectKeyFromObject(teamBetaObj), teamBetaObj))).
					To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should be ready", func() {
			err := k8sClient.Create(ctx, groupObj)
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the Group is Ready")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(groupObj), groupObj)
				g.Expect(err).NotTo(HaveOccurred())
				By("Checking the conditions")
				ExpectObjConditionToBeReady(g, groupObj)
			}, timeout, interval).Should(Succeed())
		})
	})
})
