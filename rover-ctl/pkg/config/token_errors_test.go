// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"encoding/base64"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
)

const (
	invalidTokenMessage = "ROVER_TOKEN is invalid or incomplete. Please check that you copied the entire team token " +
		"and are using the correct token for your environment."
	secretValue   = "super-secret-value"
	clientIdValue = "my-client-id-value"
)

func encodeTestToken(fields map[string]any) string {
	raw, err := json.Marshal(fields)
	Expect(err).ToNot(HaveOccurred())
	return "test--my-group--my-team." + base64.StdEncoding.EncodeToString(raw)
}

func validTokenFields() map[string]any {
	return map[string]any{
		"environment":   "test",
		"client_id":     clientIdValue,
		"client_secret": secretValue,
		"token_url":     "https://auth.example.com/token",
		"server_url":    "https://server.example.com",
		"generated_at":  1716474294,
	}
}

func withFields(change func(map[string]any)) string {
	f := validTokenFields()
	change(f)
	return encodeTestToken(f)
}

var _ = Describe("Token configuration errors", func() {
	BeforeEach(func() {
		// Configure through env vars like a customer would; viper.Set("token")
		// would shadow the nested "token.url" key.
		viper.Reset()
		GinkgoT().Setenv("ROVER_TOKEN", "")
		GinkgoT().Setenv("ROVER_SERVER_URL", "")
		GinkgoT().Setenv("ROVER_TOKEN_URL", "")
		config.Initialize()
		DeferCleanup(func() {
			viper.Reset()
			config.Initialize()
		})
	})

	DescribeTable("shows only the friendly message for unusable tokens",
		func(token func() string, env map[string]string) {
			for k, v := range env {
				GinkgoT().Setenv(k, v)
			}
			GinkgoT().Setenv("ROVER_TOKEN", token())

			_, err := config.GetToken()
			Expect(err).To(HaveOccurred())
			// cmd/root.go prints errors.Cause(err) to the customer.
			Expect(errors.Cause(err).Error()).To(Equal(invalidTokenMessage))
		},
		Entry("missing authentication URL", func() string {
			return withFields(func(f map[string]any) { delete(f, "token_url") })
		}, nil),
		Entry("invalid authentication URL", func() string {
			return withFields(func(f map[string]any) { f["token_url"] = "not-a-url" })
		}, nil),
		Entry("missing server URL", func() string {
			return withFields(func(f map[string]any) { delete(f, "server_url") })
		}, nil),
		Entry("invalid server URL", func() string {
			return withFields(func(f map[string]any) { f["server_url"] = "server-host" })
		}, nil),
		Entry("unparsable server URL in token", func() string {
			return withFields(func(f map[string]any) { f["server_url"] = "://bad" })
		}, nil),
		Entry("unparsable server URL override", func() string {
			return encodeTestToken(validTokenFields())
		}, map[string]string{"ROVER_SERVER_URL": "://bad"}),
		Entry("invalid authentication URL override", func() string {
			return encodeTestToken(validTokenFields())
		}, map[string]string{"ROVER_TOKEN_URL": "bad"}),
		Entry("both URLs missing", func() string {
			return withFields(func(f map[string]any) { delete(f, "token_url"); delete(f, "server_url") })
		}, nil),
		Entry("missing credentials and metadata", func() string {
			return withFields(func(f map[string]any) {
				delete(f, "client_id")
				delete(f, "client_secret")
				delete(f, "generated_at")
			})
		}, nil),
		Entry("missing team prefix", func() string {
			raw, _ := json.Marshal(validTokenFields())
			return "." + base64.StdEncoding.EncodeToString(raw)
		}, nil),
		Entry("malformed base64", func() string { return "test--g--t.%%%not-base64" }, nil),
		Entry("not JSON", func() string {
			return "test--g--t." + base64.StdEncoding.EncodeToString([]byte("garbage"))
		}, nil),
		Entry("no separator", func() string { return "truncated-token" }, nil),
	)

	It("does not leak supplied values or internal names", func() {
		GinkgoT().Setenv("ROVER_TOKEN_URL", "bad-auth-override")
		GinkgoT().Setenv("ROVER_TOKEN", withFields(func(f map[string]any) { delete(f, "server_url") }))
		_, err := config.GetToken()
		msg := errors.Cause(err).Error()
		for _, s := range []string{"ROVER_TOKEN_URL", "ROVER_SERVER_URL", "token_url", "server_url",
			"Key:", "tag", "bad-auth-override", secretValue, clientIdValue} {
			Expect(msg).ToNot(ContainSubstring(s))
		}
	})

	It("keeps validation errors recognisable as ErrTokenValidation", func() {
		GinkgoT().Setenv("ROVER_TOKEN", withFields(func(f map[string]any) { delete(f, "token_url") }))
		_, err := config.GetToken()
		Expect(errors.Is(err, config.ErrTokenValidation)).To(BeTrue())
	})

	It("keeps the missing-token error unchanged", func() {
		_, err := config.GetToken()
		Expect(err).To(BeIdenticalTo(config.ErrTokenNotSet))
	})

	It("accepts valid overrides for missing URLs", func() {
		GinkgoT().Setenv("ROVER_SERVER_URL", "https://override.example.com")
		GinkgoT().Setenv("ROVER_TOKEN_URL", "https://auth-override.example.com/token")
		GinkgoT().Setenv("ROVER_TOKEN", withFields(func(f map[string]any) { delete(f, "token_url"); delete(f, "server_url") }))
		token, err := config.GetToken()
		Expect(err).ToNot(HaveOccurred())
		Expect(token.ServerUrl).To(Equal("https://override.example.com/rover/api"))
		Expect(token.TokenUrl).To(Equal("https://auth-override.example.com/token"))
	})

	It("lets valid overrides take precedence over invalid token URLs", func() {
		GinkgoT().Setenv("ROVER_SERVER_URL", "https://override.example.com")
		GinkgoT().Setenv("ROVER_TOKEN_URL", "https://auth-override.example.com/token")
		GinkgoT().Setenv("ROVER_TOKEN", withFields(func(f map[string]any) { f["token_url"] = "bad"; f["server_url"] = "bad" }))
		_, err := config.GetToken()
		Expect(err).ToNot(HaveOccurred())
	})
})
