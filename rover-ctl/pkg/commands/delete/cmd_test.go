// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package delete_test

import (
	"bytes"
	"context"
	"errors"
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

		Context("when deleting multiple resources", func() {
			var errA, errC error

			BeforeEach(func() {
				Expect(os.WriteFile(yamlFile, []byte(`apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: rover-a
spec: {}
---
apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: rover-b
spec: {}
---
apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: rover-c
spec: {}
`), 0o644)).To(Succeed())
				errA = errors.New("delete a refused")
				errC = errors.New("wait c timed out")
				gone := &common.ObjectStatusResponse{Gone: true}
				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Delete(mock.Anything, mock.MatchedBy(func(o types.Object) bool { return o.GetName() == "rover-a" })).Return(errA).Once()
				mockHandler.EXPECT().Delete(mock.Anything, mock.MatchedBy(func(o types.Object) bool { return o.GetName() != "rover-a" })).Return(nil).Maybe()
				mockHandler.EXPECT().WaitForDeleted(mock.Anything, "rover-b").Return(gone, nil).Maybe()
				mockHandler.EXPECT().WaitForDeleted(mock.Anything, "rover-c").Return(nil, errC).Maybe()
			})

			It("should process all resources and report all failures at the end without --fail-fast", func() {
				cmd.SetArgs([]string{"--file", yamlFile})

				err := cmd.Execute()

				Expect(err).To(MatchError(errA))
				Expect(err).To(MatchError(errC))
				Expect(err.Error()).To(ContainSubstring(`Rover "rover-a"`))
				Expect(err.Error()).To(ContainSubstring(`Rover "rover-c"`))
				Expect(err.Error()).To(ContainSubstring("failed to delete 2 resource(s)"))
				mockHandler.AssertCalled(GinkgoT(), "WaitForDeleted", mock.Anything, "rover-b")
				mockHandler.AssertCalled(GinkgoT(), "WaitForDeleted", mock.Anything, "rover-c")
			})

			It("should stop at the first failure with --fail-fast", func() {
				cmd.SetArgs([]string{"--file", yamlFile, "--fail-fast"})

				err := cmd.Execute()

				Expect(err).To(MatchError(errA))
				Expect(errors.Is(err, errC)).To(BeFalse())
				mockHandler.AssertNumberOfCalls(GinkgoT(), "Delete", 1)
				mockHandler.AssertNotCalled(GinkgoT(), "WaitForDeleted", mock.Anything, mock.Anything)
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
