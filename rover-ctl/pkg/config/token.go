// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	ErrInvalidTokenFormat = errors.New("invalid token format")
	ErrMalformedBase64    = errors.New("failed to decode base64 token")
	ErrTokenNotSet        = errors.New("token is not set in configuration")
	ErrTokenParseFailed   = errors.New("failed to parse token")
	ErrTokenValidation    = errors.New("token validation failed")
)

// Global validator instance
var validate = validator.New()

// tokenContextKey is a custom type for context keys to avoid collisions
type tokenContextKey string

const tokenKey tokenContextKey = "token"

type Token struct {
	Prefix      string `json:"-"`
	Environment string `json:"environment" validate:"required"`
	Group       string `json:"group" validate:"required"`
	Team        string `json:"team" validate:"required"`

	ClientId     string `json:"client_id" validate:"required"`
	ClientSecret string `json:"client_secret" validate:"required"`
	TokenUrl     string `json:"token_url" validate:"required,url"`
	ServerUrl    string `json:"server_url" validate:"required,url"`
	GeneratedAt  int64  `json:"generated_at" validate:"required"`
}

func GetToken() (*Token, error) {
	tokenStr := viper.GetString("token")
	if tokenStr == "" {
		return nil, ErrTokenNotSet
	}

	token, err := ParseToken(tokenStr)
	if err != nil {
		return nil, errors.Wrap(err, ErrTokenParseFailed.Error())
	}

	return token, nil
}

// ParseToken decodes and parses the token string into a Token struct.
// It will apply the server and token URLs from the token itself if present,
// otherwise it falls back to the configuration values in viper.
// It ensures that the server URL has the correct base path appended.
func ParseToken(tokenStr string) (*Token, error) {
	var prefix, b64Value string
	if strings.Contains(tokenStr, ".") {
		parts := strings.SplitN(tokenStr, ".", 2)
		if len(parts) != 2 {
			return nil, ErrInvalidTokenFormat
		}
		prefix = parts[0]
		b64Value = parts[1]
	}

	var token Token

	value, err := base64.StdEncoding.DecodeString(b64Value)
	if err != nil {
		return nil, ErrMalformedBase64
	}

	err = json.Unmarshal(value, &token)
	if err != nil {
		return nil, ErrInvalidTokenFormat
	}

	token.Prefix = prefix
	token.fillPrefixInfo()

	var serverURL *url.URL
	if token.ServerUrl == "" {
		serverURL, err = url.Parse(viper.GetString(ConfigKeyServerURL))
		if err != nil {
			return nil, errors.Wrap(ErrTokenValidation, "cannot find server-URL")
		}
	} else {
		serverURL, err = url.Parse(token.ServerUrl)
		if err != nil {
			return nil, errors.Wrap(ErrTokenValidation, "invalid server URL in token")
		}
	}
	ensureCorrectBasePath(serverURL, viper.GetString("server.baseUrl"))
	token.ServerUrl = serverURL.String()

	if token.TokenUrl == "" {
		token.TokenUrl = viper.GetString(ConfigKeyTokenURL)
	}

	// Validate the token after parsing and setting all fields
	if err := token.Validate(); err != nil {
		return nil, err
	}

	return &token, nil
}

func (t *Token) GeneratedString() string {
	if t.GeneratedAt == 0 {
		return "unknown"
	}
	timezone := time.FixedZone("GMT", 0)
	return time.Unix(t.GeneratedAt, 0).In(timezone).Format(time.RFC3339)
}

func (t *Token) TimeSinceGenerated() string {
	if t.GeneratedAt == 0 {
		return "unknown"
	}
	timezone := time.FixedZone("GMT", 0)
	tokenTime := time.Unix(t.GeneratedAt, 0).In(timezone)
	delta := time.Since(tokenTime).Abs()

	// Very Recent
	if delta < 5*time.Minute {
		return "just now"
	}

	if delta < time.Hour {
		minutes := int(delta.Minutes())
		return fmt.Sprintf("%d minutes ago", minutes)
	}

	// Hours
	if delta < 24*time.Hour {
		hours := int(delta.Hours())
		return fmt.Sprintf("%d hour(s) ago", hours)
	}

	// Days
	if delta < 48*time.Hour {
		return "yesterday"
	}
	if delta < 7*24*time.Hour {
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("%d day(s) ago", days)
	}

	// Weeks
	if delta < 30*24*time.Hour {
		weeks := int(delta.Hours() / 24 / 7)
		return fmt.Sprintf("%d week(s) ago", weeks)
	}

	// Months
	if delta < 60*24*time.Hour {
		return "1 month ago"
	}
	if delta < 365*24*time.Hour {
		months := int(delta.Hours() / 24 / 30)
		return fmt.Sprintf("%d months ago", months)
	}

	// Years
	years := int(delta.Hours() / 24 / 365)
	return fmt.Sprintf("%d year(s) ago", years)
}

func (t *Token) fillPrefixInfo() {
	if t.Prefix == "" {
		return
	}

	parts := strings.Split(t.Prefix, "--")
	if len(parts) < 3 {
		return
	}

	t.Environment = parts[0]
	t.Group = parts[1]
	t.Team = parts[2]
}

func (t *Token) Validate() error {
	if err := validate.Struct(t); err != nil {
		return errors.Wrap(err, ErrTokenValidation.Error())
	}
	return nil
}

func (t *Token) Encode() (string, error) {
	value, err := json.Marshal(t)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal token")
	}

	b64Value := base64.StdEncoding.EncodeToString(value)
	if t.Prefix != "" {
		return fmt.Sprintf("%s.%s", t.Prefix, b64Value), nil
	}
	return b64Value, nil
}

func NewContext(ctx context.Context, token *Token) context.Context {
	if token == nil {
		return ctx
	}

	return context.WithValue(ctx, tokenKey, token)
}

func FromContext(ctx context.Context) (*Token, bool) {
	token, ok := ctx.Value(tokenKey).(*Token)
	return token, ok
}

func FromContextOrDie(ctx context.Context) *Token {
	token, ok := FromContext(ctx)
	if !ok {
		panic("token not found in context")
	}
	return token
}

// ensureCorrectBasePath checks and sets the URL path to the expected base path if not already set.
// It modifies the provided url.URL in place.
func ensureCorrectBasePath(url *url.URL, expectedPath string) {
	if expectedPath == "" {
		return
	}
	// If the path is empty or just "/", set it to the expected base path
	if url.Path == "" || url.Path == "/" {
		// set the expected base path
		url.Path = expectedPath
	}
	// check if the path already ends with the expected base path
	if !strings.HasSuffix(url.Path, expectedPath) {
		// append the expected base path
		url.Path = strings.TrimRight(url.Path, "/") + expectedPath
	}
}
