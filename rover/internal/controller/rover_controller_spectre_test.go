// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rover Controller Spectre Watch", Ordered, func() {
	const (
		spectreRoverName  = "spectre-watch-test"
		providerAppName   = "provider-app"
		providerGroupName = "provider-group"
		providerTeamName  = "provider-team"
		apiBasePath       = "/eni/provider/v1"
	)

	spectreTimeout := 10 * time.Second
	_ = spectreTimeout
	ctx := context.Background()

	providerTeamNamespace := testEnvironment + "--" + providerGroupName + "--" + providerTeamName

	spectreTypeNamespacedName := client.ObjectKey{
		Name:      spectreRoverName,
		Namespace: teamNamespace,
	}

	var team *organizationv1.Team
	var providerTeam *organizationv1.Team

	BeforeAll(func() {
		By("Creating the environment namespace")
		createNamespace(testEnvironment)

		By("Creating the consumer team")
		team = newTeam(teamName, group)
		err := k8sClient.Create(ctx, team)
		if !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		createNamespace(teamNamespace)

		By("Creating the provider team and namespace")
		providerTeam = newTeam(providerTeamName, providerGroupName)
		err = k8sClient.Create(ctx, providerTeam)
		if !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		createNamespace(providerTeamNamespace)

		By("Creating the provider Application")
		providerApp := &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      providerAppName,
				Namespace: teamNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey:          testEnvironment,
					config.BuildLabelKey("application"): labelutil.NormalizeValue(providerAppName),
					config.BuildLabelKey("team"):        labelutil.NormalizeValue(providerTeamName),
					config.BuildLabelKey("zone"):        labelutil.NormalizeValue(testEnvironment),
				},
			},
			Spec: applicationv1.ApplicationSpec{
				Team:          providerTeamName,
				TeamEmail:     "provider@mail.de",
				Secret:        "provider-secret",
				NeedsClient:   false,
				NeedsConsumer: false,
			},
		}
		Expect(k8sClient.Create(ctx, providerApp)).To(Succeed())
	})

	AfterEach(func() {
		resource := &roverv1.Rover{}
		err := k8sClient.Get(ctx, spectreTypeNamespacedName, resource)
		if errors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())

		By("Cleanup the Rover")
		secretManagerMock.EXPECT().DeleteApplication(mock.Anything, testEnvironment, group+"--"+teamName, spectreRoverName).Return(nil).Times(1)
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, spectreTypeNamespacedName, resource)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}, spectreTimeout, interval).Should(Succeed())
	})

	AfterAll(func() {
		By("Cleanup provider Application")
		providerApp := &applicationv1.Application{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: providerAppName, Namespace: teamNamespace}, providerApp); err == nil {
			Expect(k8sClient.Delete(ctx, providerApp)).To(Succeed())
		}

		By("Cleanup provider Team")
		if providerTeam != nil {
			_ = k8sClient.Delete(ctx, providerTeam)
		}
	})

	Context("Spectre child readiness triggers Rover re-reconciliation", func() {
		It("should re-reconcile Rover when SpectreApplication status changes", func() {
			spec := roverv1.RoverSpec{
				Zone:         testEnvironment,
				ClientSecret: "topsecret",
				Listeners: []roverv1.RoverListener{
					{
						Consumer:    spectreRoverName,
						Provider:    providerAppName,
						ApiBasePath: apiBasePath,
					},
				},
			}

			rover := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      spectreRoverName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: spec,
			}

			By("Creating the Rover with a listener")
			Expect(k8sClient.Create(ctx, rover)).To(Succeed())

			By("Waiting for SpectreApplication and Listener to be created")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				g.Expect(fetchedRover.Status.SpectreApplications).To(HaveLen(1))
				g.Expect(fetchedRover.Status.SpectreListeners).To(HaveLen(1))
			}, spectreTimeout, interval).Should(Succeed())

			By("Manually setting SpectreApplication and Listener to Ready (no downstream controllers in this envtest)")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())

				app := &spectrev1.SpectreApplication{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      fetchedRover.Status.SpectreApplications[0].Name,
					Namespace: fetchedRover.Status.SpectreApplications[0].Namespace,
				}, app)).To(Succeed())
				app.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ready"))
				g.Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())

				listener := &spectrev1.Listener{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      fetchedRover.Status.SpectreListeners[0].Name,
					Namespace: fetchedRover.Status.SpectreListeners[0].Namespace,
				}, listener)).To(Succeed())
				listener.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ready"))
				g.Expect(k8sClient.Status().Update(ctx, listener)).To(Succeed())
			}, spectreTimeout, interval).Should(Succeed())

			By("Waiting for Rover to become Ready")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				readyCond := findCondition(fetchedRover.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, spectreTimeout, interval).Should(Succeed())

			By("Setting SpectreApplication to NotReady")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				app := &spectrev1.SpectreApplication{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      fetchedRover.Status.SpectreApplications[0].Name,
					Namespace: fetchedRover.Status.SpectreApplications[0].Namespace,
				}, app)).To(Succeed())
				app.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, "child not ready"))
				g.Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())
			}, spectreTimeout, interval).Should(Succeed())

			By("Verifying Rover becomes NotReady via watch-triggered re-reconciliation")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				readyCond := findCondition(fetchedRover.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			}, spectreTimeout, interval).Should(Succeed())

			By("Setting SpectreApplication back to Ready")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				app := &spectrev1.SpectreApplication{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      fetchedRover.Status.SpectreApplications[0].Name,
					Namespace: fetchedRover.Status.SpectreApplications[0].Namespace,
				}, app)).To(Succeed())
				app.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Ready"))
				g.Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())
			}, spectreTimeout, interval).Should(Succeed())

			By("Verifying Rover returns to Ready via watch-triggered re-reconciliation")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				readyCond := findCondition(fetchedRover.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, spectreTimeout, interval).Should(Succeed())
		})
	})
})

// findCondition returns the condition with the given type, or nil if not found.
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
