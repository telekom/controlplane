// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur_test

import (
	"context"
	"fmt"
	"io"

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
	})
})
