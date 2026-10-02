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
				Namespace: providerTeamNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey:          testEnvironment,
					config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(providerAppName),
					config.BuildLabelKey("team"):        labelutil.NormalizeLabelValue(providerTeamName),
					config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(testEnvironment),
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
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: providerAppName, Namespace: providerTeamNamespace}, providerApp); err == nil {
			Expect(k8sClient.Delete(ctx, providerApp)).To(Succeed())
		}

		By("Cleanup provider Team")
		if providerTeam != nil {
			_ = k8sClient.Delete(ctx, providerTeam)
		}

		By("Cleanup consumer Team")
		if team != nil {
			_ = k8sClient.Delete(ctx, team)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(team), &organizationv1.Team{})
				return errors.IsNotFound(err)
			}, spectreTimeout, interval).Should(BeTrue())
		}
	})

	Context("Listener provider given as full application ID", func() {
		It("should resolve the provider in the team namespace of the ID", func() {
			rover := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      spectreRoverName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: roverv1.RoverSpec{
					Zone:         testEnvironment,
					ClientSecret: "topsecret",
					Listeners: []roverv1.RoverListener{
						{
							Consumer:    spectreRoverName,
							Provider:    providerGroupName + "--" + providerTeamName + "--" + providerAppName,
							ApiBasePath: apiBasePath + "/full-id",
						},
					},
				},
			}

			By("Creating the Rover with a full-ID provider")
			Expect(k8sClient.Create(ctx, rover)).To(Succeed())

			By("Waiting for the Listener to reference the provider Application")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				g.Expect(fetchedRover.Status.SpectreListeners).To(HaveLen(1))

				listener := &spectrev1.Listener{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      fetchedRover.Status.SpectreListeners[0].Name,
					Namespace: fetchedRover.Status.SpectreListeners[0].Namespace,
				}, listener)).To(Succeed())
				g.Expect(listener.Spec.Provider.Name).To(Equal(providerAppName))
				g.Expect(listener.Spec.Provider.Namespace).To(Equal(providerTeamNamespace))
			}, spectreTimeout, interval).Should(Succeed())
		})
	})

	Context("One listener cannot be resolved", func() {
		It("should keep reconciling the other listeners and report the Rover as Blocked", func() {
			listenerAPath := apiBasePath + "/blocked-a"
			listenerCPath := apiBasePath + "/blocked-c"
			getListener := func(g Gomega, name string) *spectrev1.Listener {
				listener := &spectrev1.Listener{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: teamNamespace}, listener)).To(Succeed())
				return listener
			}
			expectBlocked := func(g Gomega, rover *roverv1.Rover) {
				readyCond := findCondition(rover.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				processingCond := findCondition(rover.Status.Conditions, condition.ConditionTypeProcessing)
				g.Expect(processingCond).NotTo(BeNil())
				g.Expect(processingCond.Reason).To(Equal(condition.ReasonBlocked))
				g.Expect(processingCond.Message).To(ContainSubstring(`application "does-not-exist" not found`))
			}

			rover := &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      spectreRoverName,
					Namespace: teamNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: roverv1.RoverSpec{
					Zone:         testEnvironment,
					ClientSecret: "topsecret",
					Listeners: []roverv1.RoverListener{
						{Consumer: spectreRoverName, Provider: providerAppName, ApiBasePath: listenerAPath},
						{Consumer: spectreRoverName, Provider: "does-not-exist", ApiBasePath: apiBasePath + "/blocked-b"},
						{Consumer: spectreRoverName, Provider: providerAppName, ApiBasePath: listenerCPath},
					},
				},
			}

			By("Creating the Rover with one unresolvable listener")
			Expect(k8sClient.Create(ctx, rover)).To(Succeed())

			By("Waiting for the resolvable Listeners and a Blocked Rover")
			var listenerAName, listenerCName string
			var listenerAUID string
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				g.Expect(fetchedRover.Status.SpectreListeners).To(HaveLen(2))
				expectBlocked(g, fetchedRover)

				listenerAName = fetchedRover.Status.SpectreListeners[0].Name
				listenerCName = fetchedRover.Status.SpectreListeners[1].Name
				listenerA := getListener(g, listenerAName)
				g.Expect(listenerA.Spec.ApiListener.ApiBasePath).To(Equal(listenerAPath))
				listenerAUID = string(listenerA.UID)
				g.Expect(getListener(g, listenerCName).Spec.ApiListener.ApiBasePath).To(Equal(listenerCPath))
			}, spectreTimeout, interval).Should(Succeed())

			By("Breaking the provider of the first listener and removing the last one")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				fetchedRover.Spec.Listeners = []roverv1.RoverListener{
					{Consumer: spectreRoverName, Provider: "does-not-exist", ApiBasePath: listenerAPath},
				}
				g.Expect(k8sClient.Update(ctx, fetchedRover)).To(Succeed())
			}, spectreTimeout, interval).Should(Succeed())

			By("Verifying the existing Listener is kept and the removed one is cleaned up")
			Eventually(func(g Gomega) {
				fetchedRover := &roverv1.Rover{}
				g.Expect(k8sClient.Get(ctx, spectreTypeNamespacedName, fetchedRover)).To(Succeed())
				g.Expect(fetchedRover.Status.SpectreListeners).To(HaveLen(1))
				g.Expect(fetchedRover.Status.SpectreListeners[0].Name).To(Equal(listenerAName))
				expectBlocked(g, fetchedRover)

				g.Expect(string(getListener(g, listenerAName).UID)).To(Equal(listenerAUID))
				err := k8sClient.Get(ctx, client.ObjectKey{Name: listenerCName, Namespace: teamNamespace}, &spectrev1.Listener{})
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}, spectreTimeout, interval).Should(Succeed())
		})
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
