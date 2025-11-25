// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
)

var _ = Describe("Token", Ordered, func() {

	BeforeAll(func() {
		config.Initialize()
	})

	It("should parse a valid token string", func() {
		tokenStr := "test--my-group--my-team.ewogICJlbnZpcm9ubWVudCIgOiAidGVzdCIsCiAgImdlbmVyYXRlZF9hdCIgOiAxNzE2NDc0Mjk0LAogICJjbGllbnRfc2VjcmV0IiA6ICJ0b3BzZWNyZXQiLAogICJjbGllbnRfaWQiIDogInRlc3QtZ3JvdXAtLXRlc3QtdGVhbS0tdGVhbS11c2VyIiwKICAidG9rZW5fdXJsIjogImh0dHBzOi8vZXhhbXBsZS5jb20vdG9rZW4iLAogICJzZXJ2ZXJfdXJsIjogImh0dHA6Ly9sb2NhbGhvc3Q6ODA4MCIKfQ=="

		token, err := config.ParseToken(tokenStr)
		Expect(err).ToNot(HaveOccurred())
		Expect(token).ToNot(BeNil())
		Expect(token.Prefix).To(Equal("test--my-group--my-team"))
		Expect(token.Environment).To(Equal("test"))
		Expect(token.Group).To(Equal("my-group"))
		Expect(token.Team).To(Equal("my-team"))

		Expect(token.ClientId).To(Equal("test-group--test-team--team-user"))
		Expect(token.ClientSecret).To(Equal("topsecret"))
		Expect(token.TokenUrl).To(Equal("https://example.com/token"))
		Expect(token.ServerUrl).To(Equal("http://localhost:8080/rover/api")) // It must have the base path appended
		Expect(token.GeneratedAt).To(BeNumerically(">", 0))
		timeSince := token.TimeSinceGenerated()
		Expect(timeSince).To(MatchRegexp(`\d+ year\(s\) ago`))
		Expect(token.GeneratedString()).To(Equal("2024-05-23T14:24:54Z"))
	})

	It("should parse a valid token string and retain the URLs in the token", func() {
		// Create a token with URLs
		tokenStr := "test--my-group--my-team.ewogICJlbnZpcm9ubWVudCIgOiAidGVzdCIsCiAgImdlbmVyYXRlZF9hdCIgOiAxNzE2NDc0Mjk0OTk3LAogICJjbGllbnRfc2VjcmV0IiA6ICJ0b3BzZWNyZXQiLAogICJjbGllbnRfaWQiIDogInRlc3QtZ3JvdXAtLXRlc3QtdGVhbS0tdGVhbS11c2VyIiwKICAidG9rZW5fdXJsIjogImh0dHBzOi8vZXhhbXBsZS5jb20vdG9rZW4iLAogICJzZXJ2ZXJfdXJsIjogImh0dHA6Ly9sb2NhbGhvc3Q6ODA4MCIKfQ=="

		// Set URLs in viper (these should not override the token URLs)
		viper.Set("server.url", "http://viper-server:9090")
		viper.Set("auth.token_url", "https://viper-auth.example.com/token")

		// Parse the token
		token, err := config.ParseToken(tokenStr)

		// Verify the token
		Expect(err).ToNot(HaveOccurred())
		Expect(token).ToNot(BeNil())
		Expect(token.Prefix).To(Equal("test--my-group--my-team"))
		Expect(token.Environment).To(Equal("test"))
		Expect(token.Group).To(Equal("my-group"))
		Expect(token.Team).To(Equal("my-team"))

		// Verify that URLs from the token are used, not from viper
		Expect(token.ServerUrl).To(Equal("http://localhost:8080/rover/api")) // It must have the base path appended
		Expect(token.TokenUrl).To(Equal("https://example.com/token"))
	})

	It("should return an error for an invalid token format", func() {
		tokenStr := "invalid-token-format"
		_, err := config.ParseToken(tokenStr)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(config.ErrInvalidTokenFormat))
	})

	It("should return an error for a malformed base64 string", func() {
		tokenStr := "test--my-group--my-team.invalid-base64"
		_, err := config.ParseToken(tokenStr)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(config.ErrMalformedBase64))
	})

	It("should return an error if the token is not set in configuration", func() {
		viper.Set("token", "")
		_, err := config.GetToken()
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(config.ErrTokenNotSet))
	})

	It("should return an error if the token cannot be parsed", func() {
		viper.Set("token", "invalid-token")
		_, err := config.GetToken()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(config.ErrTokenParseFailed.Error()))
		Expect(err.Error()).To(ContainSubstring(config.ErrInvalidTokenFormat.Error()))
	})

	It("should return validation error for token with missing required fields", func() {
		// Token with missing client_id and client_secret
		tokenStr := "test--my-group--my-team.ewogICJlbnZpcm9ubWVudCIgOiAidGVzdCIsCiAgImdlbmVyYXRlZF9hdCIgOiAxNzE2NDc0Mjk0OTk3LAogICJ0b2tlbl91cmwiOiAiaHR0cHM6Ly9leGFtcGxlLmNvbS90b2tlbiIsCiAgInNlcnZlcl91cmwiOiAiaHR0cDovL2xvY2FsaG9zdDo4MDgwIgp9"

		_, err := config.ParseToken(tokenStr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(config.ErrTokenValidation.Error()))
	})

	It("should use viper values when URLs are missing from token", func() {
		// Create a token without URLs
		tokenStr := "test--my-group--my-team.ewogICJlbnZpcm9ubWVudCIgOiAidGVzdCIsCiAgImdlbmVyYXRlZF9hdCIgOiAxNzE2NDc0Mjk0OTk3LAogICJjbGllbnRfc2VjcmV0IiA6ICJ0b3BzZWNyZXQiLAogICJjbGllbnRfaWQiIDogInRlc3QtZ3JvdXAtLXRlc3QtdGVhbS0tdGVhbS11c2VyIgp9"

		// Set URLs in viper
		viper.Set(config.ConfigKeyServerURL, "http://viper-server:9090/prefix")
		viper.Set(config.ConfigKeyTokenURL, "https://viper-auth.example.com/token")

		// Parse the token
		token, err := config.ParseToken(tokenStr)

		// Verify the token
		Expect(err).ToNot(HaveOccurred())
		Expect(token).ToNot(BeNil())

		// Verify that URLs are populated from viper
		Expect(token.ServerUrl).To(Equal("http://viper-server:9090/prefix/rover/api")) // It must have the base path appended
		Expect(token.TokenUrl).To(Equal("https://viper-auth.example.com/token"))
	})

})
