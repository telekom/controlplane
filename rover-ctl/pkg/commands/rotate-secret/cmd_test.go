// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rotatesecret_test

import (
	"bytes"
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"

	rotatesecret "github.com/telekom/controlplane/rover-ctl/pkg/commands/rotate-secret"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rotate-Secret Command", func() {
	var (
		cmd              *cobra.Command
		mockResetHandler *mocks.MockResetSecretHandler
		stdout           *bytes.Buffer
		stderr           *bytes.Buffer
	)

	BeforeEach(func() {
		// Initialize logger
		log.SetGlobalLogger(log.NewLogger())

		// Set up viper defaults
		viper.SetDefault("log.format", "json")
		viper.SetDefault("log.level", "info")
		viper.SetDefault("output.format", "json")

		// Set up token
		token := &config.Token{
			Prefix:       "test-env--test-group--test-team",
			ClientId:     "test-client",
			ClientSecret: "test-secret",
			Environment:  "test-env",
			Group:        "test-group",
			Team:         "test-team",
			TokenUrl:     "https://example.com/token",
			ServerUrl:    "https://api.example.com",
			GeneratedAt:  1754992778,
		}

		encodedToken, err := token.Encode()
		Expect(err).NotTo(HaveOccurred())
		viper.Set("token", encodedToken)

		// Register mock handlers
		mockResetHandler = mocks.NewMockResetSecretHandler(GinkgoT())
		handlers.RegisterHandler("Rover", "tcp.ei.telekom.de/v1", mockResetHandler)

		// Set up command output capture
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}

		// Create command instance
		cmd = rotatesecret.NewCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetContext(context.Background())
	})

	AfterEach(func() {
		viper.Reset()
		handlers.ResetRegistryForTest()
	})

	Describe("NewCommand", func() {
		It("should create a new command with the correct properties", func() {
			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Use).To(Equal("rotate-secret"))
			Expect(cmd.Short).To(Equal("Rotate a secret"))

			appFlag := cmd.Flags().Lookup("application")
			Expect(appFlag).NotTo(BeNil())
			Expect(appFlag.Shorthand).To(Equal("a"))

			nameFlag := cmd.Flags().Lookup("name")
			Expect(nameFlag).NotTo(BeNil())
			Expect(nameFlag.Shorthand).To(Equal("n"))
		})
	})

	Describe("Run", func() {
		Context("when rotating a secret", func() {
			BeforeEach(func() {
				handlers.ResetRegistryForTest()
				handlers.RegisterHandler("Rover", "tcp.ei.telekom.de/v1", mockResetHandler)

				mockResetHandler.EXPECT().ResetSecret(mock.AnythingOfType("*context.valueCtx"), "test-app").Return(&v0.SecretRotationStatusResponse{
					ClientId:        "new-client-id",
					ClientSecret:    "new-client-secret",
					ProcessingState: "done",
					OverallStatus:   "complete",
				}, nil).Once()

				mockResetHandler.On("Priority").Return(100).Maybe()
			})

			It("should rotate the secret successfully", func() {
				cmd.SetArgs([]string{"--application", "test-app"})

				err := cmd.Execute()

				Expect(err).NotTo(HaveOccurred())
				Expect(stdout.String()).To(ContainSubstring("new-client-id"))
				Expect(stdout.String()).To(ContainSubstring("new-client-secret"))

				mockResetHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when no handler is found for resource", func() {
			BeforeEach(func() {
				handlers.ResetRegistryForTest()
			})

			It("should return an error", func() {
				cmd.SetArgs([]string{"--application", "test-app"})

				err := cmd.Execute()

				Expect(err).To(HaveOccurred())
				Expect(stderr.String()).To(ContainSubstring("failed to get rover handler"))
			})
		})
	})
})
