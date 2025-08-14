// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base_test

import (
	"bytes"
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
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

		Context("when fail-fast is true", func() {
			It("should return an error", func() {
				// Set fail-fast to true
				baseCmd.FailFast = true

				// Handle an error
				testErr := errors.New("test error")
				result := baseCmd.HandleError(testErr, "process test")

				// Verify the error is returned
				Expect(result).To(HaveOccurred())
				Expect(result.Error()).To(ContainSubstring("failed to process test"))

				// Verify error was written to stderr
				Expect(stderr.String()).NotTo(BeEmpty())
			})
		})

		Context("when fail-fast is false", func() {
			It("should log the error but not return it", func() {
				// Set fail-fast to false
				baseCmd.FailFast = false

				// Handle an error
				testErr := errors.New("test error")
				result := baseCmd.HandleError(testErr, "process test")

				// Verify no error is returned
				Expect(result).NotTo(HaveOccurred())

				// Verify error was written to stderr
				Expect(stderr.String()).NotTo(BeEmpty())
			})
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
