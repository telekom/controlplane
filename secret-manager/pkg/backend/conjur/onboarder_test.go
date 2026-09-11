// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/cyberark/conjur-api-go/conjurapi"
	conjurapi_response "github.com/cyberark/conjur-api-go/conjurapi/response"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Conjur Onboarder", func() {

	var writeAPI *mocks.MockConjurAPI
	var writerBackend *mocks.MockBackend[conjur.ConjurSecretId, backend.DefaultSecret[conjur.ConjurSecretId]]

	BeforeEach(func() {
		writeAPI = mocks.NewMockConjurAPI(GinkgoT())
		writerBackend = mocks.NewMockBackend[conjur.ConjurSecretId, backend.DefaultSecret[conjur.ConjurSecretId]](GinkgoT())
	})

	Context("Onboarder Implementation", func() {

		It("should create a new Conjur Onboarder", func() {
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			Expect(conjurOnboarder).ToNot(BeNil())
		})
	})

	Context("Onboard Environment", func() {

		It("should onboard an environment", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"

			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane", mock.Anything).Return(nil, nil)

			res, err := conjurOnboarder.OnboardEnvironment(ctx, env)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
		})

		It("should return an error if loading policy fails", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"

			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !policy\n  id: test-env\n  body:\n  - !variable zones\n"))
				return nil, fmt.Errorf("failed to load policy")
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane", mock.Anything).RunAndReturn(runAndReturn)

			res, err := conjurOnboarder.OnboardEnvironment(ctx, env)
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should delete an environment", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"
			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !delete\n  record: !policy test-env\n"))
				return nil, nil
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePatch, "controlplane", mock.Anything).RunAndReturn(runAndReturn)

			err := conjurOnboarder.DeleteEnvironment(ctx, env)
			Expect(err).ToNot(HaveOccurred())
		})

	})

	Context("Onboard Team", func() {

		It("should onboard a team", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"
			const teamId = "test-team"

			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !policy\n  id: test-team\n  body:\n  - !variable clientSecret\n  - !variable teamToken\n"))
				return nil, nil
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env", mock.Anything).RunAndReturn(runAndReturn)

			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).Return(backend.DefaultSecret[conjur.ConjurSecretId]{}, nil).Times(2)

			res, err := conjurOnboarder.OnboardTeam(ctx, env, teamId, backend.WithStrategy(backend.StrategyMerge))
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
		})

		It("should delete a team", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)

			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !delete\n  record: !policy test-team\n"))
				return nil, nil
			}

			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePatch, "controlplane/test-env", mock.Anything).RunAndReturn(runAndReturn)

			err := conjurOnboarder.DeleteTeam(ctx, "test-env", "test-team")
			Expect(err).ToNot(HaveOccurred())

		})

		It("should onboard a team with defined secret", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"
			const teamId = "test-team"

			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !policy\n  id: test-team\n  body:\n  - !variable clientSecret\n  - !variable teamToken\n"))
				return nil, nil
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env", mock.Anything).RunAndReturn(runAndReturn)

			clientSecretCreated := false
			teamTokenCreated := false

			runAndReturnSecret := func(ctx context.Context, secretId conjur.ConjurSecretId, secretValue backend.SecretValue, opts ...backend.WriteOption) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
				if regexp.MustCompile(`^test-env:test-team::clientSecret:(.+)$`).MatchString(secretId.String()) {
					Expect(secretId.String()).To(MatchRegexp("test-env:test-team::clientSecret:.+"))
					Expect(secretValue.Value()).To(Equal("topsecret"))
					clientSecretCreated = true
					return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
				}
				if regexp.MustCompile(`^test-env:test-team::teamToken:(.+)$`).MatchString(secretId.String()) {
					Expect(secretId.String()).To(MatchRegexp("test-env:test-team::teamToken:.+"))
					Expect(secretValue.Value()).To((Equal("thisismyteamtoken")))
					teamTokenCreated = true
					return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
				}

				return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
			}

			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(runAndReturnSecret).Times(2)

			res, err := conjurOnboarder.OnboardTeam(ctx, env, teamId,
				backend.WithSecretValue("teamToken", backend.String("thisismyteamtoken")),
				backend.WithSecretValue("clientSecret", backend.String("topsecret")),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(clientSecretCreated).To(BeTrue())
			Expect(teamTokenCreated).To(BeTrue())

			Expect(res.SecretRefs()).To(HaveKeyWithValue("clientSecret", MatchRegexp("test-env:test-team::clientSecret:.+")))
			Expect(res.SecretRefs()).To(HaveKeyWithValue("teamToken", MatchRegexp("test-env:test-team::teamToken:.+")))
			Expect(res.SecretRefs()).To(HaveLen(2))

		})
	})

	Context("Onboard Application", func() {

		It("should onboard an application", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			env := "test-env"
			teamId := "test-team"
			appId := "test-app"

			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !policy\n  id: test-app\n  body:\n  - !variable clientSecret\n  - !variable rotatedClientSecret\n  - !variable externalSecrets\n"))
				return nil, nil
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env/test-team", mock.Anything).RunAndReturn(runAndReturn)

			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).Return(backend.DefaultSecret[conjur.ConjurSecretId]{}, nil)

			res, err := conjurOnboarder.OnboardApplication(ctx, env, teamId, appId, backend.WithStrategy(backend.StrategyMerge))
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
		})

		It("should delete an application", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			env := "test-env"
			teamId := "test-team"
			appId := "test-app"

			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !delete\n  record: !policy test-app\n"))
				return nil, nil
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePatch, "controlplane/test-env/test-team", mock.Anything).RunAndReturn(runAndReturn)

			err := conjurOnboarder.DeleteApplication(ctx, env, teamId, appId)
			Expect(err).ToNot(HaveOccurred())

		})

		It("should fail on unknown onboarding secret", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			env := "test-env"
			teamId := "test-team"
			appId := "test-app"

			runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
				buf, err := io.ReadAll(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(buf)).To(Equal("\n- !policy\n  id: test-app\n  body:\n  - !variable clientSecret\n  - !variable rotatedClientSecret\n  - !variable externalSecrets\n"))
				return nil, nil
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env/test-team", mock.Anything).RunAndReturn(runAndReturn)

			res, err := conjurOnboarder.OnboardApplication(ctx, env, teamId, appId, backend.WithSecretValue("extraSecret1", backend.String("topsecret")))
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("Forbidden: secret extraSecret1 is not allowed for onboarding"))
		})
	})

	It("should onboard an application with additional secrets", func() {
		ctx := context.Background()
		conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
		env := "test-env"
		teamId := "test-team"
		appId := "test-app"

		runAndReturn := func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
			buf, err := io.ReadAll(r)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(buf)).To(Equal("\n- !policy\n  id: test-app\n  body:\n  - !variable clientSecret\n  - !variable rotatedClientSecret\n  - !variable externalSecrets\n"))
			return nil, nil
		}
		writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env/test-team", mock.Anything).RunAndReturn(runAndReturn)

		clientSecretCreated := false
		rotatedClientSecretCreated := false
		externalSecretsCreated := false

		runAndReturnSecret := func(ctx context.Context, secretId conjur.ConjurSecretId, secretValue backend.SecretValue, opts ...backend.WriteOption) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
			if regexp.MustCompile(`^test-env:test-team:test-app:clientSecret:(.+)$`).MatchString(secretId.String()) {
				Expect(secretId.String()).To(MatchRegexp("test-env:test-team:test-app:clientSecret:.+"))
				Expect(secretValue.AllowChange()).To(BeFalse())
				Expect(secretValue.Value()).To(Not(BeEmpty()))
				clientSecretCreated = true
				return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
			}

			if regexp.MustCompile(`^test-env:test-team:test-app:rotatedClientSecret:(.+)$`).MatchString(secretId.String()) {
				Expect(secretValue.AllowChange()).To(BeFalse())
				Expect(secretValue.Value()).To(Equal("NOT_USED"))
				rotatedClientSecretCreated = true
				return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
			}

			Expect(externalSecretsCreated).To(BeFalse())
			Expect(secretId.String()).To(MatchRegexp("test-env:test-team:test-app:externalSecrets:.+"))
			Expect(secretValue.AllowChange()).To(BeTrue())
			Expect(secretValue.Value()).To(Equal(`{"key1":"value1","key2":"value2"}`))
			externalSecretsCreated = true
			return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
		}

		writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(runAndReturnSecret).Times(3)

		res, err := conjurOnboarder.OnboardApplication(ctx, env, teamId, appId,
			backend.WithSecretValue("externalSecrets/key1", backend.String("value1")),
			backend.WithSecretValue("externalSecrets/key2", backend.String("value2")),
			backend.WithStrategy(backend.StrategyMerge),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(clientSecretCreated).To(BeTrue())
		Expect(rotatedClientSecretCreated).To(BeTrue())
		Expect(externalSecretsCreated).To(BeTrue())

		Expect(res.SecretRefs()).To(HaveKeyWithValue("clientSecret", MatchRegexp("test-env:test-team:test-app:clientSecret:.+")))
		Expect(res.SecretRefs()).To(HaveKeyWithValue("rotatedClientSecret", MatchRegexp("test-env:test-team:test-app:rotatedClientSecret:.+")))
		Expect(res.SecretRefs()).To(HaveKey("externalSecrets"))
		Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key1", MatchRegexp("test-env:test-team:test-app:externalSecrets/key1:.*")))
		Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key2", MatchRegexp("test-env:test-team:test-app:externalSecrets/key2:.*")))
		Expect(res.SecretRefs()).To(HaveLen(5))
	})

	Context("Strategy Support", func() {

		It("merge strategy should set ALL allowed secrets with merge write option (team)", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"
			const teamId = "test-team"

			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env", mock.Anything).Return(nil, nil)

			// Both clientSecret and teamToken are Set (ALL allowed secrets)
			secretsSet := map[string]bool{}
			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, id conjur.ConjurSecretId, sv backend.SecretValue, opts ...backend.WriteOption) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
					secretsSet[id.Path()] = true
					if id.Path() == "clientSecret" {
						Expect(sv.Value()).To(Equal("my-secret"))
						Expect(sv.AllowChange()).To(BeTrue())
					}
					if id.Path() == "teamToken" {
						// teamToken not provided by user, so it uses InitialString default
						Expect(sv.AllowChange()).To(BeFalse())
					}
					return backend.NewDefaultSecret(id, sv.Value()), nil
				},
			).Times(2)

			res, err := conjurOnboarder.OnboardTeam(ctx, env, teamId,
				backend.WithSecretValue("clientSecret", backend.String("my-secret")),
				backend.WithStrategy(backend.StrategyMerge),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(secretsSet).To(HaveKey("clientSecret"))
			Expect(secretsSet).To(HaveKey("teamToken"))
			Expect(res.SecretRefs()).To(HaveKey("clientSecret"))
			Expect(res.SecretRefs()).To(HaveKey("teamToken"))
		})

		It("replace strategy should set ALL allowed secrets with replace write option (team)", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"
			const teamId = "test-team"

			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env", mock.Anything).Return(nil, nil)

			// Both clientSecret and teamToken are Set (ALL allowed secrets, no Delete)
			secretsSet := map[string]bool{}
			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, id conjur.ConjurSecretId, sv backend.SecretValue, opts ...backend.WriteOption) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
					secretsSet[id.Path()] = true
					return backend.NewDefaultSecret(id, sv.Value()), nil
				},
			).Times(2)

			res, err := conjurOnboarder.OnboardTeam(ctx, env, teamId,
				backend.WithSecretValue("clientSecret", backend.String("my-secret")),
				backend.WithStrategy(backend.StrategyReplace),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(secretsSet).To(HaveKey("clientSecret"))
			Expect(secretsSet).To(HaveKey("teamToken"))
			Expect(res.SecretRefs()).To(HaveKey("clientSecret"))
			Expect(res.SecretRefs()).To(HaveKey("teamToken"))
		})

		It("strategy should be forwarded to Set via WriteOption (application)", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			env := "test-env"
			teamId := "test-team"
			appId := "test-app"

			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env/test-team", mock.Anything).Return(nil, nil)

			// Both clientSecret and externalSecrets are Set
			secretsSet := map[string]bool{}
			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, id conjur.ConjurSecretId, sv backend.SecretValue, opts ...backend.WriteOption) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
					secretsSet[id.Path()] = true
					return backend.NewDefaultSecret(id, sv.Value()), nil
				},
			).Times(3)

			res, err := conjurOnboarder.OnboardApplication(ctx, env, teamId, appId,
				backend.WithSecretValue("externalSecrets/key1", backend.String("value1")),
				backend.WithStrategy(backend.StrategyMerge),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(secretsSet).To(HaveKey("clientSecret"))
			Expect(secretsSet).To(HaveKey("rotatedClientSecret"))
			Expect(secretsSet).To(HaveKey("externalSecrets"))
			Expect(res.SecretRefs()).To(HaveKey("clientSecret"))
			Expect(res.SecretRefs()).To(HaveKey("rotatedClientSecret"))
			Expect(res.SecretRefs()).To(HaveKey("externalSecrets"))
		})

		It("no strategy (default) should set ALL allowed secrets (team)", func() {
			ctx := context.Background()
			conjurOnboarder := conjur.NewOnboarder(writeAPI, writerBackend)
			const env = "test-env"
			const teamId = "test-team"

			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env", mock.Anything).Return(nil, nil)

			// Even with no strategy, ALL allowed secrets are Set (no Delete)
			secretsSet := map[string]bool{}
			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, id conjur.ConjurSecretId, sv backend.SecretValue, opts ...backend.WriteOption) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
					secretsSet[id.Path()] = true
					return backend.NewDefaultSecret(id, sv.Value()), nil
				},
			).Times(2)

			res, err := conjurOnboarder.OnboardTeam(ctx, env, teamId,
				backend.WithSecretValue("clientSecret", backend.String("my-secret")),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(secretsSet).To(HaveKey("clientSecret"))
			Expect(secretsSet).To(HaveKey("teamToken"))
			Expect(res.SecretRefs()).To(HaveKey("clientSecret"))
			Expect(res.SecretRefs()).To(HaveKey("teamToken"))
		})
	})

	Context("Concurrent Onboarding with Bouncer", func() {

		It("should preserve all zones sub-keys when concurrent OnboardEnvironment calls use merge strategy", func() {
			const concurrency = 10
			const env = "test-env"

			// In-memory store simulating the Conjur variable storage.
			// Key = variableId (e.g. "controlplane/test-env/zones"), Value = current secret value.
			store := make(map[string]string)
			var storeMu sync.Mutex

			// Create mock APIs backed by the in-memory store
			readAPIConcurrent := mocks.NewMockConjurAPI(GinkgoT())
			writeAPIConcurrent := mocks.NewMockConjurAPI(GinkgoT())

			readAPIConcurrent.EXPECT().RetrieveSecret(mock.Anything).RunAndReturn(
				func(variableId string) ([]byte, error) {
					storeMu.Lock()
					defer storeMu.Unlock()
					val, ok := store[variableId]
					if !ok {
						return nil, &conjurapi_response.ConjurError{Code: 404, Message: "Not Found"}
					}
					return []byte(val), nil
				},
			)

			writeAPIConcurrent.EXPECT().AddSecret(mock.Anything, mock.Anything).RunAndReturn(
				func(variableId, value string) error {
					storeMu.Lock()
					defer storeMu.Unlock()
					store[variableId] = value
					return nil
				},
			)

			writeAPIConcurrent.EXPECT().LoadPolicy(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
				func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
					_, _ = io.ReadAll(r)
					return nil, nil
				},
			)

			// Create a real ConjurBackend with bouncer (not a mock)
			locker := bouncer.NewLocker("test-onboard-env")
			realBackend := conjur.NewBackend(writeAPIConcurrent, readAPIConcurrent).WithBouncer(locker)

			// Create the onboarder with the real backend as its secretWriter and shared bouncer
			conjurOnboarder := conjur.NewOnboarder(writeAPIConcurrent, realBackend).WithBouncer(locker)

			ctx := context.Background()
			errs := make(chan error, concurrency)
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					_, err := conjurOnboarder.OnboardEnvironment(ctx, env,
						backend.WithSecretValue(fmt.Sprintf("zones/foo%d", idx), backend.String(fmt.Sprintf("bar%d", idx))),
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

			// Verify the zones variable contains ALL sub-keys from all concurrent calls
			storeMu.Lock()
			zonesValue := store["controlplane/test-env/zones"]
			storeMu.Unlock()

			Expect(zonesValue).ToNot(BeEmpty())

			var zonesMap map[string]string
			Expect(json.Unmarshal([]byte(zonesValue), &zonesMap)).To(Succeed())
			Expect(zonesMap).To(HaveLen(concurrency))
			for i := 0; i < concurrency; i++ {
				Expect(zonesMap).To(HaveKeyWithValue(fmt.Sprintf("foo%d", i), fmt.Sprintf("bar%d", i)))
			}
		})

		It("should handle concurrent OnboardTeam calls with bouncer without errors", func() {
			const concurrency = 10
			const env = "test-env"
			const teamId = "test-team"

			// Create dedicated mocks for the concurrent test
			writeAPIConcurrent := mocks.NewMockConjurAPI(GinkgoT())
			writerBackendConcurrent := mocks.NewMockBackend[conjur.ConjurSecretId, backend.DefaultSecret[conjur.ConjurSecretId]](GinkgoT())

			locker := bouncer.NewLocker("test-onboard")
			conjurOnboarder := conjur.NewOnboarder(writeAPIConcurrent, writerBackendConcurrent).WithBouncer(locker)

			// LoadPolicy should be called once per concurrent call (serialized by bouncer)
			writeAPIConcurrent.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env", mock.Anything).
				RunAndReturn(func(pm conjurapi.PolicyMode, s string, r io.Reader) (*conjurapi.PolicyResponse, error) {
					// Drain the reader to avoid issues with reuse
					_, _ = io.ReadAll(r)
					return nil, nil
				}).Times(concurrency)

			// Set is called twice per onboard (clientSecret + teamToken), serialized
			writerBackendConcurrent.EXPECT().Set(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				RunAndReturn(func(ctx context.Context, id conjur.ConjurSecretId, sv backend.SecretValue, opts ...backend.WriteOption) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
					return backend.NewDefaultSecret(id, sv.Value()), nil
				}).Times(concurrency * 2)

			ctx := context.Background()
			errs := make(chan error, concurrency)
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					_, err := conjurOnboarder.OnboardTeam(ctx, env, teamId,
						backend.WithSecretValue("clientSecret", backend.String(fmt.Sprintf("secret-%d", idx))),
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
		})
	})
})
