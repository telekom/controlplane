// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resource_test

import (
	"bytes"
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/resource"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
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
			Expect(subCmds).To(HaveLen(3))
			Expect(findSubcommand(subCmds, "delete")).NotTo(BeNil())

			// Check get command
			getCmd := findSubcommand(subCmds, "get")
			Expect(getCmd).NotTo(BeNil())

			// Check list command
			listCmd := findSubcommand(subCmds, "list")
			Expect(listCmd).NotTo(BeNil())
		})
	})

	DescribeTable("using the default API version",
		func(subcommand string) {
			handlers.RegisterHandler("TestKind", "tcp.ei.telekom.de/v1", mockHandler)
			resourceObj := &types.UnstructuredObject{
				Content: map[string]any{
					"apiVersion": "tcp.ei.telekom.de/v1",
					"kind":       "TestKind",
					"metadata": map[string]any{
						"name": "test-resource",
					},
				},
			}

			args := []string{subcommand, "--kind", "TestKind"}
			switch subcommand {
			case "get":
				args = append(args, "--name", "test-resource")
				mockHandler.EXPECT().Get(mock.AnythingOfType("*context.valueCtx"), "test-resource").Return(resourceObj, nil).Once()
			case "list":
				mockHandler.EXPECT().List(mock.AnythingOfType("*context.valueCtx")).Return([]any{resourceObj}, nil).Once()
			}

			cmd.SetArgs(args)
			Expect(cmd.Execute()).To(Succeed())
			Expect(stdout.String()).To(ContainSubstring("test-resource"))
			Expect(stdout.String()).To(ContainSubstring("tcp.ei.telekom.de/v1"))
		},
		Entry("get without --api-version", "get"),
		Entry("list without --api-version", "list"),
	)

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
			It("should only log the error", func() {
				// Set args for command with non-existent handler
				cmd.SetArgs([]string{
					"get",
					"--kind", "NonExistentKind",
					"--api-version", "test.api/v1",
					"--name", "test-resource",
				})

				// Run the command
				err := cmd.Execute()

				// Verify no error is returned
				Expect(err).ToNot(HaveOccurred())
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
			It("should only log the error", func() {
				// Set args for command with non-existent handler
				cmd.SetArgs([]string{
					"list",
					"--kind", "NonExistentKind",
					"--api-version", "test.api/v1",
				})

				// Run the command
				err := cmd.Execute()

				// Verify no error is returned
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
	Describe("legacy and modern forms", func() {
		var resourceObj *types.UnstructuredObject

		goneStatus := func() types.ObjectStatus {
			return &common.ObjectStatusResponse{
				OverallStatus:   types.OverallStatusComplete,
				ProcessingState: types.ProcessingStateDone,
				Gone:            true,
			}
		}

		// expectNotice asserts that only the deprecated selector flags print a notice, and only to stderr.
		expectNotice := func(args []string, subcommand string) {
			Expect(stdout.String()).NotTo(ContainSubstring("deprecated"))
			if args[0] == subcommand {
				Expect(stderr.String()).NotTo(ContainSubstring("deprecated"))
				return
			}
			Expect(stderr.String()).To(ContainSubstring(`Flag "--` + subcommand + `" is deprecated and will be removed in June 2027, use "roverctl resource ` + subcommand + `" instead.`))
		}

		BeforeEach(func() {
			handlers.RegisterHandler("TestKind", "tcp.ei.telekom.de/v1", mockHandler)
			resourceObj = &types.UnstructuredObject{Content: map[string]any{
				"apiVersion": "tcp.ei.telekom.de/v1",
				"kind":       "TestKind",
				"metadata":   map[string]any{"name": "test-resource"},
			}}
			mockHandler.EXPECT().Priority().Return(100).Maybe()
		})

		DescribeTable("get",
			func(args ...string) {
				mockHandler.EXPECT().Get(mock.Anything, "test-resource").Return(resourceObj, nil).Once()
				cmd.SetArgs(args)
				Expect(cmd.Execute()).To(Succeed())
				Expect(stdout.String()).To(ContainSubstring("test-resource"))
				expectNotice(args, "get")
			},
			Entry("legacy --get --name", "--kind", "TestKind", "--get", "--name", "test-resource"),
			Entry("legacy --get -n", "--get", "--kind=TestKind", "-n", "test-resource"),
			Entry("modern get --name", "get", "--kind", "TestKind", "--name", "test-resource"),
			Entry("modern get -n", "get", "--kind", "TestKind", "-n", "test-resource"),
		)

		DescribeTable("list",
			func(args ...string) {
				mockHandler.EXPECT().List(mock.Anything).Return([]any{resourceObj}, nil).Once()
				cmd.SetArgs(args)
				Expect(cmd.Execute()).To(Succeed())
				Expect(stdout.String()).To(ContainSubstring("test-resource"))
				expectNotice(args, "list")
			},
			Entry("legacy --list", "--kind", "TestKind", "--list"),
			Entry("legacy --list ignores --name", "--kind", "TestKind", "--list", "--name", "ignored"),
			Entry("modern list", "list", "--kind", "TestKind"),
		)

		DescribeTable("delete",
			func(args ...string) {
				deleter := registerNameDeleter(mockHandler, "tcp.ei.telekom.de/v1", nil)
				mockHandler.EXPECT().WaitForDeleted(mock.Anything, "test-resource").Return(goneStatus(), nil).Once()
				cmd.SetArgs(args)
				Expect(cmd.Execute()).To(Succeed())
				Expect(deleter.names).To(Equal([]string{"test-resource"}))
				expectNotice(args, "delete")
			},
			Entry("legacy --delete --name", "--kind", "TestKind", "--delete", "--name", "test-resource"),
			Entry("modern delete --name", "delete", "--kind", "TestKind", "--name", "test-resource"),
			Entry("modern delete -n", "delete", "--kind", "TestKind", "-n", "test-resource"),
		)

		It("uses the given api version for delete", func() {
			deleter := registerNameDeleter(mockHandler, "test.api/v1", nil)
			mockHandler.EXPECT().WaitForDeleted(mock.Anything, "test-resource").Return(goneStatus(), nil).Once()
			cmd.SetArgs([]string{"delete", "--kind", "TestKind", "--api-version", "test.api/v1", "--name", "test-resource"})
			Expect(cmd.Execute()).To(Succeed())
			Expect(deleter.names).To(Equal([]string{"test-resource"}))
		})

		DescribeTable("returns delete failures without --fail-fast and does not wait",
			func(args ...string) {
				registerNameDeleter(mockHandler, "tcp.ei.telekom.de/v1", errors.New("connection refused"))
				cmd.SetArgs(args)
				err := cmd.Execute()
				Expect(err).To(MatchError(ContainSubstring(`failed to delete TestKind "test-resource": connection refused`)))
				mockHandler.AssertNotCalled(GinkgoT(), "WaitForDeleted", mock.Anything, mock.Anything)
			},
			Entry("legacy", "--kind", "TestKind", "--delete", "--name", "test-resource"),
			Entry("modern", "delete", "--kind", "TestKind", "--name", "test-resource"),
		)

		DescribeTable("returns wait failures without --fail-fast",
			func(args ...string) {
				registerNameDeleter(mockHandler, "tcp.ei.telekom.de/v1", nil)
				mockHandler.EXPECT().WaitForDeleted(mock.Anything, "test-resource").Return(nil, errors.New("timed out")).Once()
				cmd.SetArgs(args)
				err := cmd.Execute()
				Expect(err).To(MatchError(ContainSubstring(`failed to wait for TestKind "test-resource" to be deleted: timed out`)))
			},
			Entry("legacy", "--kind", "TestKind", "--delete", "--name", "test-resource"),
			Entry("modern", "delete", "--kind", "TestKind", "--name", "test-resource"),
		)

		It("fails delete when the handler cannot delete by name", func() {
			cmd.SetArgs([]string{"delete", "--kind", "TestKind", "--name", "test-resource"})
			Expect(cmd.Execute()).To(MatchError(ContainSubstring("deleting TestKind by name is not supported")))
		})

		It("fails delete when no handler is registered for the kind", func() {
			cmd.SetArgs([]string{"--kind", "UnknownKind", "--delete", "--name", "test-resource"})
			err := cmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot delete UnknownKind"))
		})

		It("prints help when no flags are given", func() {
			cmd.SetArgs([]string{})
			Expect(cmd.Execute()).To(Succeed())
			Expect(stdout.String()).To(ContainSubstring("Usage:"))
		})

		DescribeTable("rejects invalid selectors without calling the server",
			func(expectedErr string, args ...string) {
				cmd.SetArgs(args)
				err := cmd.Execute()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErr))
			},
			Entry("no selector", "exactly one of --list, --get or --delete", "--kind", "TestKind"),
			Entry("--get and --delete", "mutually exclusive", "--kind", "TestKind", "--get", "--delete", "--name", "x"),
			Entry("--list and --delete", "mutually exclusive", "--kind", "TestKind", "--list", "--delete", "--name", "x"),
			Entry("legacy missing kind", "--kind must not be empty", "--list"),
			Entry("legacy empty kind", "--kind must not be empty", "--kind", "", "--get", "--name", "x"),
			Entry("legacy empty api version", "--api-version must not be empty", "--kind", "TestKind", "--api-version", "", "--list"),
			Entry("legacy get missing name", "--name must not be empty", "--kind", "TestKind", "--get"),
			Entry("legacy delete missing name", "--name must not be empty", "--kind", "TestKind", "--delete"),
			Entry("legacy delete blank name", "--name must not be empty", "--kind", "TestKind", "--delete", "--name", "  "),
			Entry("positional argument", "unknown command", "something", "--list", "--kind", "TestKind"),
			Entry("modern get missing name", "--name must not be empty", "get", "--kind", "TestKind"),
			Entry("modern get empty name", "--name must not be empty", "get", "--kind", "TestKind", "--name", ""),
			Entry("modern delete missing kind", "--kind must not be empty", "delete", "--name", "x"),
			Entry("modern delete empty kind", "--kind must not be empty", "delete", "--kind", "", "--name", "x"),
			Entry("modern delete missing name", "--name must not be empty", "delete", "--kind", "TestKind"),
			Entry("modern delete empty name", "--name must not be empty", "delete", "--kind", "TestKind", "--name", ""),
			Entry("modern list empty kind", "--kind must not be empty", "list", "--kind", " "),
		)
	})
})

// nameDeletingHandler adds DeleteByName to the generated mock, which only covers handlers.ResourceHandler.
type nameDeletingHandler struct {
	*mocks.MockResourceHandler
	err   error
	names []string
}

func (h *nameDeletingHandler) DeleteByName(_ context.Context, name string) error {
	h.names = append(h.names, name)
	return h.err
}

func registerNameDeleter(mockHandler *mocks.MockResourceHandler, apiVersion string, err error) *nameDeletingHandler {
	h := &nameDeletingHandler{MockResourceHandler: mockHandler, err: err}
	handlers.RegisterHandler("TestKind", apiVersion, h)
	return h
}

// Helper function to find a subcommand by name
func findSubcommand(commands []*cobra.Command, name string) *cobra.Command {
	for _, cmd := range commands {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
