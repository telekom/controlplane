// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
)

var _ = Describe("BaseCommand", func() {
	BeforeEach(func() {
		// Initialize logger
		log.SetGlobalLogger(log.NewLogger())
	})

	Describe("NewCommand", func() {
		It("should create a new command with the provided values", func() {
			// Create a new command
			baseCmd := base.NewCommand("test", "Test command", "Test command long description")

			// Verify the command properties
			Expect(baseCmd).NotTo(BeNil())
			Expect(baseCmd.Cmd).NotTo(BeNil())
			Expect(baseCmd.Cmd.Use).To(Equal("test"))
			Expect(baseCmd.Cmd.Short).To(Equal("Test command"))
			Expect(baseCmd.Cmd.Long).To(Equal("Test command long description"))
		})
	})

	Describe("Logger", func() {
		It("should return a logger with the command name", func() {
			// Create a new command
			baseCmd := base.NewCommand("test", "Test command", "Test command long description")

			// Get the logger
			logger := baseCmd.Logger()

			// Verify the logger is not nil
			Expect(logger).NotTo(BeNil())
			Expect(logger.IsZero()).To(BeFalse())
		})
	})

	Describe("HandleError", func() {
		var (
			baseCmd *base.BaseCommand
			stderr  *bytes.Buffer
		)

		BeforeEach(func() {
			// Create a new command with captured stderr
			baseCmd = base.NewCommand("test", "Test command", "Test command long description")
			stderr = &bytes.Buffer{}
			baseCmd.Cmd.SetErr(stderr)
		})

		DescribeTable("should print and return the error regardless of fail-fast",
			func(failFast bool) {
				baseCmd.FailFast = failFast
				testErr := errors.New("test error")

				result := baseCmd.HandleError(testErr, "process test")

				Expect(result).To(MatchError(testErr))
				Expect(result.Error()).To(ContainSubstring("failed to process test"))
				Expect(stderr.String()).To(ContainSubstring("test error"))
			},
			Entry("with fail-fast", true),
			Entry("without fail-fast", false),
		)
	})

	Describe("ForEach", func() {
		var (
			baseCmd   *base.BaseCommand
			stderr    *bytes.Buffer
			errA      = errors.New("a failed")
			errC      = errors.New("c failed")
			processed []string
			op        func(string) error
		)

		BeforeEach(func() {
			baseCmd = base.NewCommand("test", "Test command", "Test command long description")
			stderr = &bytes.Buffer{}
			baseCmd.Cmd.SetErr(stderr)
			processed = nil
			op = func(item string) error {
				processed = append(processed, item)
				switch item {
				case "a":
					return errA
				case "c":
					return errC
				}
				return nil
			}
		})

		It("should return nil when no item fails", func() {
			Expect(base.ForEach(baseCmd, "process", []string{"b", "d"}, op)).To(Succeed())
			Expect(processed).To(Equal([]string{"b", "d"}))
		})

		It("should process all items and join all failures without fail-fast", func() {
			err := base.ForEach(baseCmd, "process", []string{"a", "b", "c", "d"}, op)

			Expect(processed).To(Equal([]string{"a", "b", "c", "d"}))
			Expect(err).To(MatchError(errA))
			Expect(err).To(MatchError(errC))
			Expect(err.Error()).To(ContainSubstring("failed to process 2 resource(s)"))
		})

		It("should stop at the first failure with fail-fast", func() {
			baseCmd.FailFast = true

			err := base.ForEach(baseCmd, "process", []string{"a", "b", "c"}, op)

			Expect(processed).To(Equal([]string{"a"}))
			Expect(err).To(MatchError(errA))
			Expect(errors.Is(err, errC)).To(BeFalse())
			Expect(err.Error()).To(ContainSubstring("failed to process 1 resource(s)"))
		})

		It("should print structured API error diagnostics only once, at the end", func() {
			apiErr := &common.ApiError{Type: "ValidationError", Status: 400, Title: "Bad Request", Detail: "invalid spec"}
			err := base.ForEach(baseCmd, "process", []string{"x", "y"}, func(item string) error {
				if item == "x" {
					return fmt.Errorf("Rover %q: %w", item, apiErr)
				}
				Expect(stderr.String()).To(BeEmpty(), "diagnostics must not be printed before all items are processed")
				return errors.New("plain failure")
			})

			Expect(err).To(MatchError(apiErr))
			Expect(strings.Count(stderr.String(), "invalid spec")).To(Equal(1))
			Expect(stderr.String()).NotTo(ContainSubstring("plain failure"))
			Expect(err.Error()).To(ContainSubstring("plain failure"))
		})
	})

	Describe("SetupToken", func() {
		var (
			baseCmd *base.BaseCommand
			token   *config.Token
		)

		BeforeEach(func() {
			token = &config.Token{
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

			// Create a new command
			baseCmd = base.NewCommand("test", "Test command", "Test command long description")
			baseCmd.Cmd.SetContext(context.Background())
		})

		Context("When token is correctly configured", func() {
			It("should set up the token in the command context", func() {
				// Setup the token
				err := baseCmd.SetupToken()

				// Verify no error occurred
				Expect(err).NotTo(HaveOccurred())

				// Verify the token was set in the context
				token, ok := config.FromContext(baseCmd.Cmd.Context())
				Expect(ok).To(BeTrue())
				Expect(token).NotTo(BeNil())
				Expect(token.ClientId).To(Equal("test-client"))
				Expect(token.ClientSecret).To(Equal("test-secret"))
				Expect(token.TokenUrl).To(Equal("https://example.com/token"))
				Expect(token.ServerUrl).To(Equal("https://api.example.com"))
				Expect(token.Group).To(Equal("test-group"))
				Expect(token.Team).To(Equal("test-team"))
			})
		})

		Context("When token is not set", func() {
			BeforeEach(func() {
				viper.Set("token", "")
			})

			It("should return an error", func() {
				// Attempt to setup the token
				err := baseCmd.SetupToken()

				// Verify an error occurred
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("token is not set in configuration"))
			})
		})
	})
})
