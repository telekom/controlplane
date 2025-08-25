// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resetsecret_test

import (
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	resetsecret "github.com/telekom/controlplane/rover-ctl/pkg/commands/reset-secret"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Reset-Secret Command", func() {
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

		// Register the mock reset handler
		// Note: MockResetSecretHandler now implements the ResourceHandler interface
		handlers.RegisterHandler("Rover", "tcp.ei.telekom.de/v1", mockResetHandler)

		// Set up command output capture
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}

		// Create command instance
		cmd = resetsecret.NewCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetContext(context.Background())
	})

	AfterEach(func() {
		// Reset viper config
		viper.Reset()

		// Reset the handlers registry
		handlers.ResetRegistryForTest()
	})

	Describe("NewCommand", func() {
		It("should create a new command with the correct properties", func() {
			// Verify command properties
			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Use).To(Equal("reset-secret"))
			Expect(cmd.Short).To(Equal("Reset a secret"))

			// Verify application flag exists
			appFlag := cmd.Flags().Lookup("application")
			Expect(appFlag).NotTo(BeNil())
			Expect(appFlag.Shorthand).To(Equal("a"))

			// Check if the flag is marked as required
			// Note: We can't check flag.Required directly, but we know it's marked as required in the code
		})
	})

	Describe("Run", func() {
		Context("when resetting a secret", func() {
			BeforeEach(func() {
				// Replace the registered handler with our mock that implements ResetSecretHandler
				handlers.ResetRegistryForTest()
				handlers.RegisterHandler("Rover", "tcp.ei.telekom.de/v1", mockResetHandler)

				// Set up mock expectations
				mockResetHandler.EXPECT().ResetSecret(mock.AnythingOfType("*context.valueCtx"), "test-app").Return("new-client-id", "new-client-secret", nil).Once()

				// We also need to implement the Priority method since it's used by the handler registry
				mockResetHandler.On("Priority").Return(100).Maybe()
			})

			It("should reset the secret successfully", func() {
				// Set args for command
				cmd.SetArgs([]string{"--application", "test-app"})

				// Run the command
				err := cmd.Execute()

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify output contains expected information
				Expect(stdout.String()).To(ContainSubstring("new-client-id"))
				Expect(stdout.String()).To(ContainSubstring("new-client-secret"))

				// Verify mock expectations
				mockResetHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when no handler is found for resource", func() {
			BeforeEach(func() {
				// Remove the handler registration
				handlers.ResetRegistryForTest()
			})

			It("should return an error", func() {
				// Set args for command
				cmd.SetArgs([]string{"--application", "test-app"})

				// Run the command
				err := cmd.Execute()

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(stderr.String()).To(ContainSubstring("failed to get rover handler"))
			})
		})
	})
})
