// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
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

			Expect(mockK8sClient.Delete(ctx, secret)).To(Succeed())
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

		It("should fail on unknown onboarding secret", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// Attempt to onboard an application with an unknown secret
			_, err := onboarder.OnboardApplication(ctx, env, teamId, appId, backend.WithSecretValue("extraSecret1", backend.String("topsecret")))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Forbidden: secret extraSecret1 is not allowed for application onboarding"))
		})

		It("should onboard an application with additional secrets", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			res, err := onboarder.OnboardApplication(ctx, env, teamId, appId,
				backend.WithSecretValue("externalSecrets/key1", backend.String("value1")),
				backend.WithSecretValue("externalSecrets/key2/sub", backend.String("value2")),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SecretRefs()).To(HaveLen(4))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("clientSecret", MatchRegexp("test-env:test-team:test-app:clientSecret:.+")))
			Expect(res.SecretRefs()).To(HaveKey("externalSecrets"))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key1", MatchRegexp("test-env:test-team:test-app:externalSecrets/key1:.+")))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key2/sub", MatchRegexp("test-env:test-team:test-app:externalSecrets/key2/sub:.+")))

			// Verify that the application secret was created
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: appId, Namespace: fmt.Sprintf("%s--%s", env, teamId)}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Data).To(HaveLen(2))
			Expect(secret.Data).To(HaveKey("clientSecret"))
			b, ok := secret.Data["externalSecrets"]
			Expect(ok).To(BeTrue())
			var v map[string]any
			err = json.Unmarshal(b, &v)
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(HaveKeyWithValue("key1", "value1"))
			Expect(v).To(HaveKeyWithValue("key2/sub", "value2"))
		})
	})
})
