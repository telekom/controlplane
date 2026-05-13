// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

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

	Context("Concurrent Onboarding", func() {

		It("should handle concurrent OnboardTeam calls without errors", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)
			const concurrency = 10

			errs := make(chan error, concurrency)
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					_, err := onboarder.OnboardTeam(ctx, env, teamId,
						backend.WithSecretValue("teamToken", backend.String(fmt.Sprintf("token-%d", idx))),
					)
					errs <- err
				}(i)
			}

			wg.Wait()
			close(errs)

			for err := range errs {
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify secret exists with valid data
			secret := &corev1.Secret{}
			err := mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(secret.Data).To(HaveKey("teamToken"))
		})

		It("should handle concurrent OnboardApplication calls without errors", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)
			const concurrency = 10

			errs := make(chan error, concurrency)
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func() {
					defer wg.Done()
					defer GinkgoRecover()
					_, err := onboarder.OnboardApplication(ctx, env, teamId, appId)
					errs <- err
				}()
			}

			wg.Wait()
			close(errs)

			for err := range errs {
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify secret exists
			secret := &corev1.Secret{}
			err := mockK8sClient.Get(ctx, client.ObjectKey{Name: appId, Namespace: fmt.Sprintf("%s--%s", env, teamId)}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKey("clientSecret"))
		})

		It("should handle concurrent OnboardApplication calls with merge strategy and sub-secrets", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)
			const concurrency = 10

			errs := make(chan error, concurrency)
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					_, err := onboarder.OnboardApplication(ctx, env, teamId, appId,
						backend.WithSecretValue(fmt.Sprintf("externalSecrets/key%d", idx), backend.String(fmt.Sprintf("val%d", idx))),
						backend.WithStrategy(backend.StrategyMerge),
					)
					errs <- err
				}(i)
			}

			wg.Wait()
			close(errs)

			for err := range errs {
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify secret exists with both expected K8s data keys
			secret := &corev1.Secret{}
			err := mockK8sClient.Get(ctx, client.ObjectKey{Name: appId, Namespace: fmt.Sprintf("%s--%s", env, teamId)}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(secret.Data["clientSecret"]).ToNot(BeEmpty())
			Expect(secret.Data).To(HaveKey("externalSecrets"))

			// With JSON-level merge, all concurrent sub-keys should be preserved
			// in the externalSecrets JSON blob.
			var parsed map[string]string
			Expect(json.Unmarshal(secret.Data["externalSecrets"], &parsed)).To(Succeed())
			Expect(parsed).To(HaveLen(concurrency))
			for i := 0; i < concurrency; i++ {
				Expect(parsed).To(HaveKeyWithValue(fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i)))
			}
		})
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

		It("should preserve existing zone entries when onboarding a new zone with merge strategy", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// Onboard environment with zone dataplane1
			_, err := onboarder.OnboardEnvironment(ctx, env,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("zones/dataplane1/event/admin/clientSecret", backend.String("dp1-admin-secret")),
				backend.WithSecretValue("zones/dataplane1/event/mesh/clientSecret", backend.String("dp1-mesh-secret")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Verify initial state: zones key contains both dataplane1 entries
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: "secrets", Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKey("zones"))

			var zonesAfterFirst map[string]string
			Expect(json.Unmarshal(secret.Data["zones"], &zonesAfterFirst)).To(Succeed())
			Expect(zonesAfterFirst).To(HaveLen(2))
			Expect(zonesAfterFirst).To(HaveKeyWithValue("dataplane1/event/admin/clientSecret", "dp1-admin-secret"))
			Expect(zonesAfterFirst).To(HaveKeyWithValue("dataplane1/event/mesh/clientSecret", "dp1-mesh-secret"))

			// Onboard environment with zone dataplane2 (merge strategy)
			_, err = onboarder.OnboardEnvironment(ctx, env,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("zones/dataplane2/event/admin/clientSecret", backend.String("dp2-admin-secret")),
				backend.WithSecretValue("zones/dataplane2/event/mesh/clientSecret", backend.String("dp2-mesh-secret")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Verify: zones key should contain ALL entries from both onboarding calls
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: "secrets", Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKey("zones"))

			var zonesAfterSecond map[string]string
			Expect(json.Unmarshal(secret.Data["zones"], &zonesAfterSecond)).To(Succeed())
			Expect(zonesAfterSecond).To(HaveLen(4))
			Expect(zonesAfterSecond).To(HaveKeyWithValue("dataplane1/event/admin/clientSecret", "dp1-admin-secret"))
			Expect(zonesAfterSecond).To(HaveKeyWithValue("dataplane1/event/mesh/clientSecret", "dp1-mesh-secret"))
			Expect(zonesAfterSecond).To(HaveKeyWithValue("dataplane2/event/admin/clientSecret", "dp2-admin-secret"))
			Expect(zonesAfterSecond).To(HaveKeyWithValue("dataplane2/event/mesh/clientSecret", "dp2-mesh-secret"))
		})

		It("should overwrite zone entries with same keys on merge", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// Onboard environment with zone dataplane1
			_, err := onboarder.OnboardEnvironment(ctx, env,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("zones/dataplane1/event/admin/clientSecret", backend.String("original-secret")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Re-onboard with a new value for the same key
			_, err = onboarder.OnboardEnvironment(ctx, env,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("zones/dataplane1/event/admin/clientSecret", backend.String("updated-secret")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Verify: the key should have the updated value
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: "secrets", Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())

			var zones map[string]string
			Expect(json.Unmarshal(secret.Data["zones"], &zones)).To(Succeed())
			Expect(zones).To(HaveLen(1))
			Expect(zones).To(HaveKeyWithValue("dataplane1/event/admin/clientSecret", "updated-secret"))
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

		It("should onboard a team with defined secret", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			res, err := onboarder.OnboardTeam(ctx, env, teamId,
				backend.WithSecretValue("teamToken", backend.String("myteamtokenvalue")),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SecretRefs()).To(HaveLen(2))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("clientSecret", MatchRegexp("test-env:test-team::clientSecret:.+")))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("teamToken", MatchRegexp("test-env:test-team::teamToken:.+")))

			// Verify that the application secret was created
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Data).To(HaveLen(2))
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(secret.Data["clientSecret"]).ToNot(BeEmpty())
			Expect(secret.Data).To(HaveKey("teamToken"))
			Expect(string(secret.Data["teamToken"])).To(Equal("myteamtokenvalue"))
		})

		It("should preserve existing secrets with explicit merge strategy", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// First onboard with a defined teamToken
			_, err := onboarder.OnboardTeam(ctx, env, teamId,
				backend.WithSecretValue("teamToken", backend.String("original-token")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Verify initial state
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveLen(2))
			Expect(string(secret.Data["teamToken"])).To(Equal("original-token"))
			originalClientSecret := string(secret.Data["clientSecret"])

			// Re-onboard with explicit merge strategy and provide a new teamToken value
			_, err = onboarder.OnboardTeam(ctx, env, teamId,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("teamToken", backend.String("updated-token")),
			)
			Expect(err).ToNot(HaveOccurred())

			// With merge:
			// - teamToken should be updated to the new user-provided value
			// - clientSecret should be preserved from existing (InitialString, AllowChange=false)
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveLen(2))
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(string(secret.Data["clientSecret"])).To(Equal(originalClientSecret))
			Expect(secret.Data).To(HaveKey("teamToken"))
			Expect(string(secret.Data["teamToken"])).To(Equal("updated-token"))
		})

		It("should replace all secrets with replace strategy", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// First onboard to create the secret with initial data
			_, err := onboarder.OnboardTeam(ctx, env, teamId,
				backend.WithSecretValue("teamToken", backend.String("original-token")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Verify initial state
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveLen(2))
			originalClientSecret := string(secret.Data["clientSecret"])

			// Manually add an extra key to the existing K8s secret to simulate
			// a key that only exists in existing data (not in the template)
			secret.Data["extraKey"] = []byte("extra-value")
			err = mockK8sClient.Update(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			// Verify extraKey exists
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveLen(3))
			Expect(secret.Data).To(HaveKey("extraKey"))

			// Re-onboard with replace strategy
			_, err = onboarder.OnboardTeam(ctx, env, teamId,
				backend.WithStrategy(backend.StrategyReplace),
				backend.WithSecretValue("teamToken", backend.String("new-token")),
			)
			Expect(err).ToNot(HaveOccurred())

			// With replace: only template keys survive, extraKey is dropped.
			// clientSecret is InitialString (AllowChange=false), so its existing value is preserved.
			// teamToken is String (AllowChange=true, user-provided), so it gets the new value.
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveLen(2))
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(string(secret.Data["clientSecret"])).To(Equal(originalClientSecret))
			Expect(secret.Data).To(HaveKey("teamToken"))
			Expect(string(secret.Data["teamToken"])).To(Equal("new-token"))
			// extraKey should be gone with replace strategy
			Expect(secret.Data).ToNot(HaveKey("extraKey"))
		})

		It("should preserve extra keys with explicit merge strategy", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// First onboard to create the secret
			_, err := onboarder.OnboardTeam(ctx, env, teamId)
			Expect(err).ToNot(HaveOccurred())

			// Manually add an extra key to the existing K8s secret
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			secret.Data["extraKey"] = []byte("extra-value")
			err = mockK8sClient.Update(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			// Re-onboard with explicit merge strategy
			_, err = onboarder.OnboardTeam(ctx, env, teamId,
				backend.WithStrategy(backend.StrategyMerge),
			)
			Expect(err).ToNot(HaveOccurred())

			// With merge: extraKey should be preserved
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveLen(3))
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(secret.Data).To(HaveKey("teamToken"))
			Expect(secret.Data).To(HaveKey("extraKey"))
			Expect(string(secret.Data["extraKey"])).To(Equal("extra-value"))
		})

		It("should drop extra keys with default strategy (replace)", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// First onboard to create the secret
			_, err := onboarder.OnboardTeam(ctx, env, teamId)
			Expect(err).ToNot(HaveOccurred())

			// Manually add an extra key to the existing K8s secret
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			secret.Data["extraKey"] = []byte("extra-value")
			err = mockK8sClient.Update(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			// Re-onboard with no strategy (default is replace)
			_, err = onboarder.OnboardTeam(ctx, env, teamId)
			Expect(err).ToNot(HaveOccurred())

			// With replace (default): extraKey should be dropped
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: teamId, Namespace: env}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveLen(2))
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(secret.Data).To(HaveKey("teamToken"))
			Expect(secret.Data).ToNot(HaveKey("extraKey"))
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
			Expect(err.Error()).To(ContainSubstring("Forbidden: secret extraSecret1 is not allowed for onboarding"))
		})

		It("should onboard an application with additional secrets", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			res, err := onboarder.OnboardApplication(ctx, env, teamId, appId,
				backend.WithSecretValue("externalSecrets/key1", backend.String("value1")),
				backend.WithSecretValue("externalSecrets/key2/sub", backend.String("value2")),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.SecretRefs()).To(HaveLen(5))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("clientSecret", MatchRegexp("test-env:test-team:test-app:clientSecret:.+")))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("rotatedClientSecret", MatchRegexp("test-env:test-team:test-app:rotatedClientSecret:.+")))
			Expect(res.SecretRefs()).To(HaveKey("externalSecrets"))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key1", MatchRegexp("test-env:test-team:test-app:externalSecrets/key1:.+")))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key2/sub", MatchRegexp("test-env:test-team:test-app:externalSecrets/key2/sub:.+")))

			// Verify that the application secret was created
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: appId, Namespace: fmt.Sprintf("%s--%s", env, teamId)}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Data).To(HaveLen(3))
			Expect(secret.Data).To(HaveKey("clientSecret"))
			Expect(secret.Data).To(HaveKey("rotatedClientSecret"))
			b, ok := secret.Data["externalSecrets"]
			Expect(ok).To(BeTrue())
			var v map[string]any
			err = json.Unmarshal(b, &v)
			Expect(err).ToNot(HaveOccurred())
			Expect(v).To(HaveKeyWithValue("key1", "value1"))
			Expect(v).To(HaveKeyWithValue("key2/sub", "value2"))
		})

		It("should merge application sub-secrets with existing ones using merge strategy", func() {
			onboarder := kubernetes.NewOnboarder(mockK8sClient)

			// First onboard with some external secrets
			_, err := onboarder.OnboardApplication(ctx, env, teamId, appId,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("externalSecrets/key1", backend.String("value1")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Second onboard with different external secrets
			_, err = onboarder.OnboardApplication(ctx, env, teamId, appId,
				backend.WithStrategy(backend.StrategyMerge),
				backend.WithSecretValue("externalSecrets/key2", backend.String("value2")),
			)
			Expect(err).ToNot(HaveOccurred())

			// Verify: both keys should be present in externalSecrets
			secret := &corev1.Secret{}
			err = mockK8sClient.Get(ctx, client.ObjectKey{Name: appId, Namespace: fmt.Sprintf("%s--%s", env, teamId)}, secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Data).To(HaveKey("externalSecrets"))

			var parsed map[string]string
			Expect(json.Unmarshal(secret.Data["externalSecrets"], &parsed)).To(Succeed())
			Expect(parsed).To(HaveLen(2))
			Expect(parsed).To(HaveKeyWithValue("key1", "value1"))
			Expect(parsed).To(HaveKeyWithValue("key2", "value2"))
		})
	})
})
