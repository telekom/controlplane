// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resource_test

import (
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/resource"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Resource Command", func() {
	var (
		cmd         *cobra.Command
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

		// Register a mock handler for testing
		mockHandler = mocks.NewMockResourceHandler(GinkgoT())
		handlers.RegisterHandler("TestKind", "test.api/v1", mockHandler)

		// Set up command output capture
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}

		// Create command instance
		cmd = resource.NewCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)

		// Set up the context with token
		ctx, err := base.SetupTokenInContext(context.Background())
		Expect(err).NotTo(HaveOccurred())
		cmd.SetContext(ctx)
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
			Expect(cmd.Use).To(Equal("resource"))
			Expect(cmd.Short).To(Equal("Manage resources"))

			// Verify subcommands
			subCmds := cmd.Commands()
			Expect(subCmds).To(HaveLen(2))

			// Check get command
			getCmd := findSubcommand(subCmds, "get")
			Expect(getCmd).NotTo(BeNil())

			// Check list command
			listCmd := findSubcommand(subCmds, "list")
			Expect(listCmd).NotTo(BeNil())
		})
	})

	Describe("Get Command", func() {

		Context("when getting a resource", func() {
			BeforeEach(func() {
				// Set up mock behavior
				resourceObj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "test.api/v1",
						"kind":       "TestKind",
						"metadata": map[string]any{
							"name": "test-resource",
						},
						"spec": map[string]any{
							"field1": "value1",
							"field2": 42,
						},
					},
				}

				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().Get(mock.AnythingOfType("*context.valueCtx"), "test-resource").Return(resourceObj, nil).Once()
			})

			It("should get the resource successfully", func() {
				// Set args for command to run the get subcommand
				cmd.SetArgs([]string{
					"get",
					"--kind", "TestKind",
					"--api-version", "test.api/v1",
					"--name", "test-resource",
				})

				// Run the command
				err := cmd.Execute()

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify output contains expected information
				Expect(stdout.String()).To(ContainSubstring("test-resource"))
				Expect(stdout.String()).To(ContainSubstring("TestKind"))
				Expect(stdout.String()).To(ContainSubstring("test.api/v1"))

				// Verify mock expectations
				mockHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when the handler is not found", func() {
			It("should return an error", func() {
				// Set args for command with non-existent handler
				cmd.SetArgs([]string{
					"get",
					"--kind", "NonExistentKind",
					"--api-version", "test.api/v1",
					"--name", "test-resource",
				})

				// Run the command
				err := cmd.Execute()

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(stderr.String()).To(ContainSubstring("no handler found"))
			})
		})
	})

	Describe("List Command", func() {

		Context("when listing resources", func() {
			BeforeEach(func() {
				// Set up mock behavior
				resource1 := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "test.api/v1",
						"kind":       "TestKind",
						"metadata": map[string]any{
							"name": "resource-1",
						},
						"spec": map[string]any{
							"field1": "value1",
						},
					},
				}

				resource2 := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "test.api/v1",
						"kind":       "TestKind",
						"metadata": map[string]any{
							"name": "resource-2",
						},
						"spec": map[string]any{
							"field1": "value2",
						},
					},
				}

				resources := []any{resource1, resource2}

				mockHandler.EXPECT().Priority().Return(100).Maybe()
				mockHandler.EXPECT().List(mock.AnythingOfType("*context.valueCtx")).Return(resources, nil).Once()
			})

			It("should list resources successfully", func() {
				// Set args for command
				cmd.SetArgs([]string{
					"list",
					"--kind", "TestKind",
					"--api-version", "test.api/v1",
				})

				// Run the command
				err := cmd.Execute()

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify output contains expected information
				Expect(stdout.String()).To(ContainSubstring("resource-1"))
				Expect(stdout.String()).To(ContainSubstring("resource-2"))

				// Verify mock expectations
				mockHandler.AssertExpectations(GinkgoT())
			})
		})

		Context("when the handler is not found", func() {
			It("should return an error", func() {
				// Set args for command with non-existent handler
				cmd.SetArgs([]string{
					"list",
					"--kind", "NonExistentKind",
					"--api-version", "test.api/v1",
				})

				// Run the command
				err := cmd.Execute()

				// Verify error
				Expect(err).To(HaveOccurred())
				Expect(stderr.String()).To(ContainSubstring("no handler found"))
			})
		})
	})
})

// Helper function to find a subcommand by name
func findSubcommand(commands []*cobra.Command, name string) *cobra.Command {
	for _, cmd := range commands {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
