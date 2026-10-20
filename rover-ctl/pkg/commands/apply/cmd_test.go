// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apply_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/apply"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Apply Command", func() {
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

		// Create a temporary directory for test files
		var err error
		tempDir, err = os.MkdirTemp("", "apply-cmd-test")
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
    - basePath: /test
      upstream: http://example.com
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
		cmd = apply.NewCommand()
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
			Expect(cmd.Use).To(Equal("apply"))
			Expect(cmd.Short).To(Equal("Apply a resource configuration"))

			// Verify file flag exists
			flag := cmd.Flags().Lookup("file")
			Expect(flag).NotTo(BeNil())
			// Note: We can't check flag.Required directly, but we know it's marked as required in the code
		})
	})

	Describe("Run", func() {
		Context("when applying a valid resource", func() {
			BeforeEach(func() {
				// Set up mock behavior for successful resource application
				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Apply(mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*types.UnstructuredObject")).Return(nil)

				// Status should be ready
				status := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusComplete,
					ProcessingState: types.ProcessingStateDone,
				}
				mockHandler.EXPECT().WaitForReady(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(status, nil)
			})

			It("should apply the resource successfully", func() {
				// Set args for command
				cmd.SetArgs([]string{"--file", yamlFile})

				// Run the command
				err := cmd.Execute()

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify mock expectations
				mockHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when applying a resource with warnings", func() {
			BeforeEach(func() {
				// Set up mock behavior
				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Apply(mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*types.UnstructuredObject")).Return(nil)

				// Status should have warnings
				warnings := []types.StatusInfo{
					{
						Message: "Some configuration is not optimal",
					},
				}
				status := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusComplete,
					ProcessingState: types.ProcessingStateDone,
					Warnings:        warnings,
				}
				mockHandler.EXPECT().WaitForReady(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(status, nil)
			})

			It("should apply the resource and show warnings", func() {
				// Set args for command
				cmd.SetArgs([]string{"--file", yamlFile})

				// Run the command
				err := cmd.Execute()

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify mock expectations
				mockHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when the final status is failed", func() {
			const diagnostic = "Scopes and basic authentication cannot be used together"

			BeforeEach(func() {
				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Apply(mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*types.UnstructuredObject")).Return(nil)

				status := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusFailed,
					ProcessingState: types.ProcessingStateFailed,
					Errors: []types.StatusInfo{{
						Cause:    "Error",
						Message:  diagnostic,
						Resource: types.ObjectRef{Kind: "Rover", Name: "test-rover"},
					}},
				}
				mockHandler.EXPECT().WaitForReady(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(status, nil)
			})

			DescribeTable("should print the diagnostics once and fail without an HTTP 500",
				func(format string, failFast bool) {
					viper.Set("log.format", format)
					args := []string{"--file", yamlFile}
					if failFast {
						args = append(args, "--fail-fast")
					}
					cmd.SetArgs(args)

					err := cmd.Execute()

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(`finished with status "failed"`))
					_, isApiErr := common.AsApiError(err)
					Expect(isApiErr).To(BeFalse())
					Expect(strings.Count(stdout.String(), diagnostic)).To(Equal(1))
					Expect(stderr.String()).NotTo(ContainSubstring(diagnostic))
					Expect(stdout.String() + stderr.String()).NotTo(ContainSubstring("InternalError"))
					Expect(stdout.String() + stderr.String()).NotTo(ContainSubstring("500"))
				},
				Entry("console output without fail-fast", "console", false),
				Entry("console output with fail-fast", "console", true),
				Entry("json output without fail-fast", "json", false),
				Entry("json output with fail-fast", "json", true),
			)
		})

		Context("when waiting fails with a genuine API error", func() {
			BeforeEach(func() {
				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Apply(mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*types.UnstructuredObject")).Return(nil)
				apiErr := &common.ApiError{Type: "InternalError", Status: 500, Title: "Internal Server Error", Detail: "boom"}
				mockHandler.EXPECT().WaitForReady(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(nil, apiErr)
			})

			expectApiErrorPrinted := func() {
				Expect(stderr.String()).To(ContainSubstring("InternalError"))
				Expect(stderr.String()).To(ContainSubstring("500"))
				Expect(stderr.String()).To(ContainSubstring("Internal Server Error"))
				Expect(stderr.String()).To(ContainSubstring("boom"))
			}

			It("should print the API error and continue without fail-fast", func() {
				cmd.SetArgs([]string{"--file", yamlFile})

				Expect(cmd.Execute()).To(Succeed())
				expectApiErrorPrinted()
			})

			It("should print the API error and fail with fail-fast", func() {
				cmd.SetArgs([]string{"--file", yamlFile, "--fail-fast"})

				err := cmd.Execute()

				Expect(err).To(HaveOccurred())
				apiErr, ok := common.AsApiError(err)
				Expect(ok).To(BeTrue())
				Expect(apiErr.Status).To(Equal(500))
				expectApiErrorPrinted()
			})
		})

		Context("when the resource file doesn't exist", func() {
			It("should return an error", func() {
				// Set args for command with non-existent file
				cmd.SetArgs([]string{"--file", filepath.Join(tempDir, "nonexistent.yaml")})

				// Run the command
				err := cmd.Execute()

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(stderr.String()).To(ContainSubstring("failed to parse files"))
			})
		})

		Context("when no handler is found for resource", func() {
			BeforeEach(func() {
				// Remove the handler registration
				handlers.ResetRegistryForTest()
			})

			It("should only log the error", func() {
				// Set args for command
				cmd.SetArgs([]string{"--file", yamlFile})

				// Run the command
				err := cmd.Execute()

				// Verify no error is returned
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
