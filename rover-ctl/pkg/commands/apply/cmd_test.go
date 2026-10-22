// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apply_test

import (
	"bytes"
	"context"
	"errors"
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

var errWrite = errors.New("stdout closed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

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

		Context("exit-code contract", func() {
			BeforeEach(func() {
				mockHandler.EXPECT().Priority().Return(100).Maybe()
			})

			writeTwoRovers := func() {
				content := `apiVersion: tcp.ei.telekom.de/v1
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
`
				Expect(os.WriteFile(yamlFile, []byte(content), 0o644)).To(Succeed())
			}

			DescribeTable("should succeed when processing ends without an actual failure",
				func(overall types.OverallStatus) {
					mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil)
					status := &common.ObjectStatusResponse{
						OverallStatus:   overall,
						ProcessingState: types.ProcessingStateDone,
						Warnings:        []types.StatusInfo{{Message: "awaiting external action"}},
					}
					mockHandler.EXPECT().WaitForReady(mock.Anything, "test-rover").Return(status, nil)

					cmd.SetArgs([]string{"--file", yamlFile})
					Expect(cmd.Execute()).To(Succeed())
				},
				Entry("blocked (e.g. approval pending)", types.OverallStatusBlocked),
				Entry("done", types.OverallStatusDone),
			)

			It("should succeed and display the blocked status when a sub-resource awaits approval", func() {
				mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil)
				// Mirrors the rover-server /status response: child Ready=False problems,
				// including normal pending approvals, are reported in errors.
				status := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusBlocked,
					ProcessingState: types.ProcessingStateDone,
					Errors: []types.StatusInfo{{
						Cause:    "ApprovalPending",
						Message:  "Approval is pending",
						Resource: types.ObjectRef{Kind: "ApiSubscription", Name: "test-sub"},
					}},
				}
				mockHandler.EXPECT().WaitForReady(mock.Anything, "test-rover").Return(status, nil)
				cmd.SetArgs([]string{"--file", yamlFile})

				Expect(cmd.Execute()).To(Succeed())
				Expect(stdout.String()).To(ContainSubstring("blocked"))
				Expect(stdout.String()).To(ContainSubstring("Approval is pending"))
			})

			DescribeTable("should fail when the final status is not terminal or is a failure",
				func(overall types.OverallStatus) {
					mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil)
					status := &common.ObjectStatusResponse{OverallStatus: overall}
					mockHandler.EXPECT().WaitForReady(mock.Anything, "test-rover").Return(status, nil)
					cmd.SetArgs([]string{"--file", yamlFile})

					err := cmd.Execute()

					Expect(err).To(MatchError(ContainSubstring(string(overall))))
				},
				Entry("failed", types.OverallStatusFailed),
				Entry("invalid", types.OverallStatusInvalid),
				Entry("processing", types.OverallStatusProcessing),
				Entry("pending", types.OverallStatusPending),
				Entry("unknown", types.OverallStatus("unexpected")),
			)

			Context("with multiple failures and a successful resource in between", func() {
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
					errA = &common.ApiError{Type: "ValidationError", Status: 400, Title: "Bad Request", Detail: "rover-a is invalid"}
					errC = errors.New("context deadline exceeded")
					ok := &common.ObjectStatusResponse{OverallStatus: types.OverallStatusComplete, ProcessingState: types.ProcessingStateDone}
					mockHandler.EXPECT().Apply(mock.Anything, mock.MatchedBy(func(o types.Object) bool { return o.GetName() == "rover-a" })).Return(errA).Once()
					mockHandler.EXPECT().Apply(mock.Anything, mock.MatchedBy(func(o types.Object) bool { return o.GetName() != "rover-a" })).Return(nil).Maybe()
					mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-b").Return(ok, nil).Maybe()
					mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-c").Return(nil, errC).Maybe()
				})

				It("should process every resource and report all failures once at the end without --fail-fast", func() {
					cmd.SetArgs([]string{"--file", yamlFile})

					err := cmd.Execute()

					Expect(err).To(MatchError(errA))
					Expect(err).To(MatchError(errC))
					Expect(err.Error()).To(ContainSubstring(`Rover "rover-a"`))
					Expect(err.Error()).To(ContainSubstring(`Rover "rover-c"`))
					Expect(err.Error()).NotTo(ContainSubstring(`Rover "rover-b"`))
					Expect(err.Error()).To(ContainSubstring("failed to apply 2 resource(s)"))
					Expect(strings.Count(stderr.String(), `"detail": "rover-a is invalid"`)).To(Equal(1))
					mockHandler.AssertCalled(GinkgoT(), "WaitForReady", mock.Anything, "rover-b")
					mockHandler.AssertCalled(GinkgoT(), "WaitForReady", mock.Anything, "rover-c")
				})

				It("should report only the first failure with --fail-fast", func() {
					cmd.SetArgs([]string{"--file", yamlFile, "--fail-fast"})

					err := cmd.Execute()

					Expect(err).To(MatchError(errA))
					Expect(errors.Is(err, errC)).To(BeFalse())
					Expect(stderr.String()).To(ContainSubstring("rover-a is invalid"))
					mockHandler.AssertNumberOfCalls(GinkgoT(), "Apply", 1)
					mockHandler.AssertNotCalled(GinkgoT(), "WaitForReady", mock.Anything, mock.Anything)
				})
			})

			Context("when printing the status fails", func() {
				BeforeEach(func() {
					cmd.SetOut(failingWriter{})
				})

				DescribeTable("should fail for otherwise non-failing statuses",
					func(overall types.OverallStatus, failFast bool) {
						mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil).Once()
						status := &common.ObjectStatusResponse{OverallStatus: overall, ProcessingState: types.ProcessingStateDone}
						mockHandler.EXPECT().WaitForReady(mock.Anything, "test-rover").Return(status, nil).Once()
						args := []string{"--file", yamlFile}
						if failFast {
							args = append(args, "--fail-fast")
						}
						cmd.SetArgs(args)
						err := cmd.Execute()
						Expect(errors.Is(err, errWrite)).To(BeTrue())
						Expect(err.Error()).To(ContainSubstring(`Rover "test-rover"`))
						Expect(err.Error()).To(ContainSubstring("failed to print status"))
					},
					Entry("blocked without --fail-fast", types.OverallStatusBlocked, false),
					Entry("blocked with --fail-fast", types.OverallStatusBlocked, true),
					Entry("done without --fail-fast", types.OverallStatusDone, false),
					Entry("done with --fail-fast", types.OverallStatusDone, true),
				)

				DescribeTable("should respect the continuation policy",
					func(failFast bool) {
						writeTwoRovers()
						blocked := &common.ObjectStatusResponse{OverallStatus: types.OverallStatusBlocked}
						ok := &common.ObjectStatusResponse{OverallStatus: types.OverallStatusComplete}
						calls := 2
						args := []string{"--file", yamlFile}
						if failFast {
							calls = 1
							args = append(args, "--fail-fast")
						} else {
							mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-b").Return(ok, nil).Once()
						}
						mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil).Times(calls)
						mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-a").Return(blocked, nil).Once()
						cmd.SetArgs(args)
						Expect(errors.Is(cmd.Execute(), errWrite)).To(BeTrue())
						mockHandler.AssertNumberOfCalls(GinkgoT(), "Apply", calls)
						mockHandler.AssertExpectations(GinkgoT())
					},
					Entry("continue without --fail-fast", false),
					Entry("stop with --fail-fast", true),
				)

				It("should preserve the original wait error", func() {
					waitErr := errors.New("resource processing failed")
					failed := &common.ObjectStatusResponse{OverallStatus: types.OverallStatusFailed}
					mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil).Once()
					mockHandler.EXPECT().WaitForReady(mock.Anything, "test-rover").Return(failed, waitErr).Once()
					cmd.SetArgs([]string{"--file", yamlFile})
					err := cmd.Execute()
					Expect(errors.Is(err, waitErr)).To(BeTrue())
					Expect(errors.Is(err, errWrite)).To(BeTrue())
				})
			})

			It("should process all objects and fail at the end when readiness fails without --fail-fast", func() {
				writeTwoRovers()
				failed := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusFailed,
					ProcessingState: types.ProcessingStateFailed,
				}
				ok := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusComplete,
					ProcessingState: types.ProcessingStateDone,
				}
				mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil).Times(2)
				mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-a").
					Return(failed, errors.New("resource processing failed")).Once()
				mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-b").Return(ok, nil).Once()

				cmd.SetArgs([]string{"--file", yamlFile})
				err := cmd.Execute()

				Expect(err).To(HaveOccurred())
				mockHandler.AssertExpectations(GinkgoT())
			})

			It("should process all objects and fail at the end when apply fails without --fail-fast", func() {
				writeTwoRovers()
				ok := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusComplete,
					ProcessingState: types.ProcessingStateDone,
				}
				mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(errors.New("bad request")).Once()
				mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil).Once()
				mockHandler.EXPECT().WaitForReady(mock.Anything, mock.Anything).Return(ok, nil).Once()

				cmd.SetArgs([]string{"--file", yamlFile})
				err := cmd.Execute()

				Expect(err).To(HaveOccurred())
				mockHandler.AssertExpectations(GinkgoT())
			})

			It("should continue with remaining objects after a non-ready status without --fail-fast", func() {
				writeTwoRovers()
				invalid := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusInvalid,
					ProcessingState: types.ProcessingStateFailed,
				}
				ok := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusComplete,
					ProcessingState: types.ProcessingStateDone,
				}
				mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil).Times(2)
				mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-a").Return(invalid, nil).Once()
				mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-b").Return(ok, nil).Once()

				cmd.SetArgs([]string{"--file", yamlFile})
				err := cmd.Execute()

				Expect(err).To(HaveOccurred())
				mockHandler.AssertExpectations(GinkgoT())
			})

			It("should stop at the first failure with --fail-fast", func() {
				writeTwoRovers()
				failed := &common.ObjectStatusResponse{
					OverallStatus:   types.OverallStatusFailed,
					ProcessingState: types.ProcessingStateFailed,
				}
				mockHandler.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil).Once()
				mockHandler.EXPECT().WaitForReady(mock.Anything, "rover-a").
					Return(failed, errors.New("resource processing failed")).Once()

				cmd.SetArgs([]string{"--file", yamlFile, "--fail-fast"})
				Expect(cmd.Execute()).To(HaveOccurred())
				mockHandler.AssertExpectations(GinkgoT())
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
