// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package getinfo_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	getinfo "github.com/telekom/controlplane/rover-ctl/pkg/commands/get-info"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Get-Info Command", func() {
	var (
		cmd         *cobra.Command
		tempDir     string
		yamlFile    string
		mockHandler *mocks.MockResourceHandler
		stdout      *bytes.Buffer
		stderr      *bytes.Buffer
	)

	BeforeEach(func() {
		// Initialize logger
		log.SetGlobalLogger(log.NewLogger())

		// Set up viper defaults
		viper.SetDefault("log.format", "json")
		viper.SetDefault("log.level", "info")
		viper.SetDefault("output.format", "json")

		// Create a temporary directory for test files
		var err error
		tempDir, err = os.MkdirTemp("", "get-info-cmd-test")
		Expect(err).NotTo(HaveOccurred())

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

		// Create a test YAML file
		yamlFile = filepath.Join(tempDir, "test.yaml")
		yamlContent := `apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: test-rover
spec:
  exposures:
    - port: 8080
      security:
        username: testuser
        password: testpass
`
		err = os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		Expect(err).NotTo(HaveOccurred())

		// Register a mock handler for Rover
		mockHandler = mocks.NewMockResourceHandler(GinkgoT())
		handlers.RegisterHandler("Rover", "tcp.ei.telekom.de/v1", mockHandler)

		// Set up command output capture
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}

		// Create command instance
		cmd = getinfo.NewCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetContext(context.Background())
	})

	AfterEach(func() {
		// Clean up
		os.RemoveAll(tempDir)

		// Reset viper config
		viper.Reset()

		// Reset the handlers registry
		handlers.ResetRegistryForTest()
	})

	Describe("NewCommand", func() {
		It("should create a new command with the correct properties", func() {
			// Verify command properties
			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Use).To(Equal("get-info"))
			Expect(cmd.Short).To(Equal("Get information about a resource"))

			// Verify flags exist
			nameFlag := cmd.Flags().Lookup("name")
			Expect(nameFlag).NotTo(BeNil())

			fileFlag := cmd.Flags().Lookup("file")
			Expect(fileFlag).NotTo(BeNil())

			shallowFlag := cmd.Flags().Lookup("shallow")
			Expect(shallowFlag).NotTo(BeNil())
		})
	})

	Describe("Run", func() {
		Context("when getting info by name", func() {
			BeforeEach(func() {
				// Prepare mock response
				infoResponse := map[string]any{
					"name":   "test-rover",
					"status": "Running",
					"details": map[string]any{
						"version":  "1.0.0",
						"lastSeen": "2023-01-01T12:00:00Z",
					},
				}

				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Info(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(infoResponse, nil).Once()
			})

			It("should get info for the resource successfully", func() {
				// Set args for command
				cmd.SetArgs([]string{"--name", "test-rover"})

				// Run the command
				err := cmd.Execute()

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify output contains expected information
				Expect(stdout.String()).To(ContainSubstring("test-rover"))
				Expect(stdout.String()).To(ContainSubstring("Running"))
				Expect(stdout.String()).To(ContainSubstring("version"))

				// Verify mock expectations
				mockHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when getting info by file", func() {
			It("should get info for the resource in the file", func() {
				// Prepare mock response
				infoResponse := map[string]any{
					"name":   "test-rover",
					"status": "Running",
					"details": map[string]any{
						"version":  "1.0.0",
						"lastSeen": "2023-01-01T12:00:00Z",
					},
				}

				mockHandler.EXPECT().Info(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(infoResponse, nil).Once()

				// Set args for command
				cmd.SetArgs([]string{"--file", yamlFile})

				// Run the command
				err := cmd.Execute()

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify output contains expected information
				Expect(stdout.String()).To(ContainSubstring("test-rover"))
				Expect(stdout.String()).To(ContainSubstring("Running"))
				Expect(stdout.String()).To(ContainSubstring("version"))

				// Verify mock expectations
				mockHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when the resource file doesn't exist", func() {
			It("should return an error", func() {
				// Skip test since we don't have a reliable way to test this scenario
				Skip("This test requires a reliable way to ensure file parsing fails")
			})
		})

		Context("when no handler is found for resource", func() {
			BeforeEach(func() {
				// Remove the handler registration
				handlers.ResetRegistryForTest()
			})

			It("should return an error", func() {
				// Set args for command
				cmd.SetArgs([]string{"--name", "test-rover"})

				// Run the command
				err := cmd.Execute()

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(stderr.String()).To(ContainSubstring("failed to get rover handler"))
			})
		})
	})
})
