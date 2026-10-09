// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
)

const (
	friendlyMessage = "ROVER_TOKEN is invalid or incomplete. Please check that you copied the entire team token " +
		"and are using the correct token for your environment."
	secretValue   = "super-secret-value"
	clientIdValue = "my-client-id-value"
)

func captureLogger() (logr.Logger, *strings.Builder) {
	var out strings.Builder
	logger := funcr.New(func(prefix, args string) {
		out.WriteString(args)
		out.WriteString("\n")
	}, funcr.Options{})
	return logger, &out
}

func tokenWith(change func(map[string]any)) string {
	fields := map[string]any{
		"environment":   "test",
		"client_id":     clientIdValue,
		"client_secret": secretValue,
		"token_url":     "https://user:pw-in-url@auth.example.com/token",
		"server_url":    "https://server.example.com",
		"generated_at":  1716474294,
	}
	change(fields)
	raw, err := json.Marshal(fields)
	Expect(err).ToNot(HaveOccurred())
	return "test--my-group--my-team." + base64.StdEncoding.EncodeToString(raw)
}

var _ = Describe("Error logging", func() {
	BeforeEach(func() {
		viper.Reset()
		GinkgoT().Setenv("ROVER_SERVER_URL", "")
		GinkgoT().Setenv("ROVER_TOKEN_URL", "")
		config.Initialize()
		DeferCleanup(func() {
			viper.Reset()
			config.Initialize()
		})
	})

	tokenError := func(token string, env map[string]string) error {
		for k, v := range env {
			GinkgoT().Setenv(k, v)
		}
		GinkgoT().Setenv("ROVER_TOKEN", token)
		_, err := config.GetToken()
		Expect(err).To(HaveOccurred())
		return err
	}

	DescribeTable("separates customer output from debug details",
		func(token string, env map[string]string, sentinel error, debugDetail string, sensitive []string) {
			err := tokenError(token, env)
			Expect(errors.Is(err, sentinel)).To(BeTrue())
			Expect(errors.Cause(err)).To(BeIdenticalTo(sentinel))

			normalLogger, normal := captureLogger()
			logError(normalLogger, err, false)
			Expect(normal.String()).To(ContainSubstring(friendlyMessage))
			Expect(normal.String()).ToNot(ContainSubstring(debugDetail))
			for _, s := range []string{"Token.", "failed rule", "ROVER_TOKEN_URL", "ROVER_SERVER_URL"} {
				Expect(normal.String()).ToNot(ContainSubstring(s))
			}

			debugLogger, debug := captureLogger()
			logError(debugLogger, err, true)
			Expect(debug.String()).To(ContainSubstring(debugDetail))
			Expect(debug.String()).To(ContainSubstring(friendlyMessage))

			for _, s := range append(sensitive, secretValue, clientIdValue, "pw-in-url") {
				Expect(normal.String()).ToNot(ContainSubstring(s))
				Expect(debug.String()).ToNot(ContainSubstring(s))
			}
		},
		Entry("missing URL keeps validator field and rule",
			tokenWith(func(f map[string]any) { delete(f, "token_url") }), nil,
			config.ErrTokenValidation, `Token.TokenUrl failed rule \"required\"`, nil),
		Entry("invalid URL keeps validator field and rule",
			tokenWith(func(f map[string]any) { f["server_url"] = "server-host-value" }), nil,
			config.ErrTokenValidation, `Token.ServerUrl failed rule \"url\"`, []string{"server-host-value"}),
		Entry("invalid URL override",
			tokenWith(func(map[string]any) {}), map[string]string{"ROVER_TOKEN_URL": "bad-override-value"},
			config.ErrTokenValidation, `Token.TokenUrl failed rule \"url\"`, []string{"bad-override-value"}),
		Entry("unparsable server URL",
			tokenWith(func(f map[string]any) { f["server_url"] = "://bad-parse-value" }), nil,
			config.ErrTokenValidation, "invalid server URL in token: missing protocol scheme", []string{"bad-parse-value"}),
		Entry("invalid port in token server URL",
			tokenWith(func(f map[string]any) { f["server_url"] = "https://usr:pw-in-url@host.example.com:99port" }), nil,
			config.ErrTokenValidation, "invalid server URL in token: invalid port", []string{"99port", "usr", "host.example.com"}),
		Entry("invalid port in server URL override",
			tokenWith(func(map[string]any) {}), map[string]string{"ROVER_SERVER_URL": "https://usr:pw-in-url@host.example.com:99port"},
			config.ErrTokenValidation, "invalid server URL override: invalid port", []string{"99port", "usr", "host.example.com"}),
		Entry("invalid escape in token server URL",
			tokenWith(func(f map[string]any) { f["server_url"] = "https://usr:pw-in-url@host.example.com/%zzpath" }), nil,
			config.ErrTokenValidation, "invalid server URL in token: invalid URL escape", []string{"%zz", "usr", "host.example.com"}),
		Entry("invalid escape in server URL override",
			tokenWith(func(map[string]any) {}), map[string]string{"ROVER_SERVER_URL": "https://usr:pw-in-url@host.example.com/%zzpath"},
			config.ErrTokenValidation, "invalid server URL override: invalid URL escape", []string{"%zz", "usr", "host.example.com"}),
		Entry("missing credentials",
			tokenWith(func(f map[string]any) { delete(f, "client_secret") }), nil,
			config.ErrTokenValidation, `Token.ClientSecret failed rule \"required\"`, nil),
		Entry("malformed base64",
			"test--g--t.abc%secretchars", nil,
			config.ErrMalformedBase64, "illegal base64 data at input byte 3", []string{"secretchars"}),
		Entry("malformed JSON",
			"test--g--t."+base64.StdEncoding.EncodeToString([]byte(`{"client_secret": xsecretx}`)), nil,
			config.ErrInvalidTokenFormat, "syntax error at offset", []string{"xsecretx"}),
		Entry("wrong JSON type",
			"test--g--t."+base64.StdEncoding.EncodeToString([]byte(`{"generated_at": "typed-secret"}`)), nil,
			config.ErrInvalidTokenFormat, `field \"generated_at\" must be of type int64`, []string{"typed-secret"}),
	)

	It("keeps format, base64 and validation sentinels distinct", func() {
		err := tokenError("test--g--t.abc%", nil)
		Expect(errors.Is(err, config.ErrMalformedBase64)).To(BeTrue())
		Expect(errors.Is(err, config.ErrInvalidTokenFormat)).To(BeFalse())
		Expect(errors.Is(err, config.ErrTokenValidation)).To(BeFalse())
	})

	It("logs non-token errors unchanged", func() {
		err := errors.Wrap(errors.New("root cause"), "outer context")

		normalLogger, normal := captureLogger()
		logError(normalLogger, err, false)
		Expect(normal.String()).To(ContainSubstring(`"error"="root cause"`))
		Expect(normal.String()).ToNot(ContainSubstring("outer context"))

		debugLogger, debug := captureLogger()
		logError(debugLogger, err, true)
		Expect(debug.String()).To(ContainSubstring("outer context: root cause"))
	})
})
