// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Kubernetes Onboarder", func() {
	var ctx context.Context
	var mockK8sClient client.Client

	const env = "test-env"
	const teamId = "test-team"
	const appId = "test-app"

	BeforeEach(func() {
		ctx = context.Background()
		mockK8sClient = NewMockK8sClient()
	})

	Context("Onboard Environment", func() {

		It("should onboard an environment", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			res, err := onboarder.OnboardEnvironment(ctx, env)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())

			// Verify that the environment secret was created
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: "secrets", Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
		})

		It("should delete an environment", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// Create the environment secret first
			_, err := onboarder.OnboardEnvironment(ctx, env)
			Expect(err).ToNot(HaveOccurred())

			// Now delete the environment
			err = onboarder.DeleteEnvironment(ctx, env)
			Expect(err).ToNot(HaveOccurred())

			// Verify that the environment secret was deleted
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: "secrets", Namespace: env}, secret)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Onboard Team", func() {

		It("should onboard a team", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			res, err := onboarder.OnboardTeam(ctx, env, teamId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())

			// Verify that the team secret was created
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
		})

		It("should delete a team", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// Create the team secret first
			_, err := onboarder.OnboardTeam(ctx, env, teamId)
			Expect(err).ToNot(HaveOccurred())

			// Now delete the team
			err = onboarder.DeleteTeam(ctx, env, teamId)
			Expect(err).ToNot(HaveOccurred())

			// Verify that the team secret was deleted
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Onboard Application", func() {
		It("should onboard an application", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			res, err := onboarder.OnboardApplication(ctx, env, teamId, appId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())

			// Verify that the application secret was created
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: appId, Namespace: fmt.Sprintf("%s--%s", env, teamId)}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
		})

		It("should delete an application", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// Create the application secret first
			_, err := onboarder.OnboardApplication(ctx, env, teamId, appId)
			Expect(err).ToNot(HaveOccurred())

			// Now delete the application
			err = onboarder.DeleteApplication(ctx, env, teamId, appId)
			Expect(err).ToNot(HaveOccurred())

			// Verify that the application secret was deleted
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: appId, Namespace: fmt.Sprintf("%s--%s", env, teamId)}, secret)
			Expect(err).To(HaveOccurred())
		})
	})
})
