// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package delete_test

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
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/delete"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Delete Command", func() {
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
		tempDir, err = os.MkdirTemp("", "delete-cmd-test")
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
		cmd = delete.NewCommand()
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
			Expect(cmd.Use).To(Equal("delete"))
			Expect(cmd.Short).To(ContainSubstring("Delete a resource"))

			// Verify file flag exists
			flag := cmd.Flags().Lookup("file")
			Expect(flag).NotTo(BeNil())

			recursiveFlag := cmd.Flags().Lookup("recursive")
			Expect(recursiveFlag).NotTo(BeNil())
			Expect(recursiveFlag.Shorthand).To(Equal("R"))
			Expect(recursiveFlag.DefValue).To(Equal("true"))
		})
	})

	Describe("Run", func() {
		Context("when deleting a valid resource", func() {
			BeforeEach(func() {
				// Set up mock behavior for successful resource deletion
				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Delete(mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*types.UnstructuredObject")).Return(nil)

				// Status should be deleted
				status := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusComplete,
					ProcessingState: types.ProcessingStateDone,
					Gone:            true,
				}
				mockHandler.EXPECT().WaitForDeleted(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(status, nil)
			})

			It("should delete the resource successfully", func() {
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

		Context("when a directory contains nested resources", func() {
			var rootDir string

			BeforeEach(func() {
				rootDir = filepath.Join(tempDir, "tree")
				nestedDir := filepath.Join(rootDir, "nested")
				Expect(os.MkdirAll(nestedDir, 0o755)).To(Succeed())
				Expect(os.Rename(yamlFile, filepath.Join(nestedDir, "test.yaml"))).To(Succeed())
			})

			It("should not discover nested files with --recursive=false", func() {
				cmd.SetArgs([]string{"--file", rootDir, "--recursive=false"})
				Expect(cmd.Execute()).To(MatchError(ContainSubstring("no valid YAML or JSON files found")))
			})

			DescribeTable("should process nested files recursively by default and with the recursive flag",
				func(flag string) {
					mockHandler.EXPECT().Priority().Return(100).Maybe()
					mockHandler.EXPECT().Delete(mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*types.UnstructuredObject")).Return(nil)
					status := &common.ObjectStatusResponse{
						OverallStatus:   types.OverallStatusComplete,
						ProcessingState: types.ProcessingStateDone,
						Gone:            true,
					}
					mockHandler.EXPECT().WaitForDeleted(mock.AnythingOfType("*context.valueCtx"), "test-rover").Return(status, nil)

					args := []string{"-f", rootDir}
					if flag != "" {
						args = append(args, flag)
					}
					cmd.SetArgs(args)
					Expect(cmd.Execute()).To(Succeed())
				},
				Entry("no flag (default)", ""),
				Entry("long flag", "--recursive"),
				Entry("long flag with explicit true", "--recursive=true"),
				Entry("shorthand flag", "-R"),
				Entry("shorthand flag with explicit true", "-R=true"),
			)

			It("should not delete anything when a nested file fails to parse", func() {
				Expect(os.WriteFile(filepath.Join(rootDir, "nested", "broken.json"), []byte("{invalid"), 0o644)).To(Succeed())

				cmd.SetArgs([]string{"-f", rootDir, "-R"})
				Expect(cmd.Execute()).To(MatchError(ContainSubstring("broken.json")))
				mockHandler.AssertNotCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
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
			})
		})
	})
})
