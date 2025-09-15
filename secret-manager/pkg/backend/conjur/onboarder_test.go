// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur_test

import (
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/cyberark/conjur-api-go/conjurapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur"
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
			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything).Return(backend.DefaultSecret[conjur.ConjurSecretId]{}, nil)

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

			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything).Return(backend.DefaultSecret[conjur.ConjurSecretId]{}, nil).Times(2)

			res, err := conjurOnboarder.OnboardTeam(ctx, env, teamId)
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

			runAndReturnSecret := func(ctx context.Context, secretId conjur.ConjurSecretId, secretValue backend.SecretValue) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
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

			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything).RunAndReturn(runAndReturnSecret).Times(2)

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
				Expect(string(buf)).To(Equal("\n- !policy\n  id: test-app\n  body:\n  - !variable clientSecret\n  - !variable externalSecrets\n"))
				return nil, nil
			}
			writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env/test-team", mock.Anything).RunAndReturn(runAndReturn)

			writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything).Return(backend.DefaultSecret[conjur.ConjurSecretId]{}, nil)

			res, err := conjurOnboarder.OnboardApplication(ctx, env, teamId, appId)
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
				Expect(string(buf)).To(Equal("\n- !policy\n  id: test-app\n  body:\n  - !variable clientSecret\n  - !variable externalSecrets\n"))
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
			Expect(string(buf)).To(Equal("\n- !policy\n  id: test-app\n  body:\n  - !variable clientSecret\n  - !variable externalSecrets\n"))
			return nil, nil
		}
		writeAPI.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/test-env/test-team", mock.Anything).RunAndReturn(runAndReturn)

		clientSecretCreated := false
		externalSecretsCreated := false

		runAndReturnSecret := func(ctx context.Context, secretId conjur.ConjurSecretId, secretValue backend.SecretValue) (backend.DefaultSecret[conjur.ConjurSecretId], error) {
			if regexp.MustCompile(`^test-env:test-team:test-app:clientSecret:(.+)$`).MatchString(secretId.String()) {
				Expect(secretId.String()).To(MatchRegexp("test-env:test-team:test-app:clientSecret:.+"))
				Expect(secretValue.AllowChange()).To(BeFalse())
				Expect(secretValue.Value()).To(Not(BeEmpty()))
				clientSecretCreated = true
				return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
			}

			Expect(externalSecretsCreated).To(BeFalse())
			Expect(secretId.String()).To(MatchRegexp("test-env:test-team:test-app:externalSecrets:.+"))
			Expect(secretValue.AllowChange()).To(BeTrue())
			Expect(secretValue.Value()).To(Equal(`{"key1":"value1","key2":"value2"}`))
			externalSecretsCreated = true
			return backend.NewDefaultSecret(secretId, secretValue.Value()), nil
		}

		writerBackend.EXPECT().Set(ctx, mock.Anything, mock.Anything).RunAndReturn(runAndReturnSecret).Times(2)

		res, err := conjurOnboarder.OnboardApplication(ctx, env, teamId, appId,
			backend.WithSecretValue("externalSecrets/key1", backend.String("value1")),
			backend.WithSecretValue("externalSecrets/key2", backend.String("value2")),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(clientSecretCreated).To(BeTrue())
		Expect(externalSecretsCreated).To(BeTrue())

		Expect(res.SecretRefs()).To(HaveKeyWithValue("clientSecret", MatchRegexp("test-env:test-team:test-app:clientSecret:.+")))
		Expect(res.SecretRefs()).To(HaveKey("externalSecrets"))
		Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key1", MatchRegexp("test-env:test-team:test-app:externalSecrets/key1:.*")))
		Expect(res.SecretRefs()).To(HaveKeyWithValue("externalSecrets/key2", MatchRegexp("test-env:test-team:test-app:externalSecrets/key2:.*")))
		Expect(res.SecretRefs()).To(HaveLen(4))
	})
})
