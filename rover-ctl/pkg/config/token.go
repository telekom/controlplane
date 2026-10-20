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
	ErrInvalidTokenFormat = errors.New(invalidTokenMessage)
	ErrMalformedBase64    = errors.New(invalidTokenMessage)
	ErrTokenNotSet        = errors.New("token is not set in configuration")
	ErrTokenParseFailed   = errors.New("failed to parse token")
	ErrTokenValidation    = errors.New(invalidTokenMessage)
)

// invalidTokenMessage is the only customer-facing message for unusable tokens.
// It must not reveal internal configuration, token fields or supplied values.
const invalidTokenMessage = "ROVER_TOKEN is invalid or incomplete. Please check that you copied the entire team token " +
	"and are using the correct token for your environment."

// Global validator instance
var validate = validator.New()

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
		return nil, errors.Wrap(ErrMalformedBase64, describeBase64Error(err))
	}

	err = json.Unmarshal(value, &token)
	if err != nil {
		return nil, errors.Wrap(ErrInvalidTokenFormat, describeJSONError(err))
	}

	token.Prefix = prefix
	token.fillPrefixInfo()

	var serverURL *url.URL
	overwriteServerURL := viper.GetString(ConfigKeyServerURL)
	if overwriteServerURL != "" {
		serverURL, err = url.Parse(overwriteServerURL)
		if err != nil {
			return nil, errors.Wrap(ErrTokenValidation, "invalid server URL override: "+describeURLError(err))
		}
	} else {
		serverURL, err = url.Parse(token.ServerUrl)
		if err != nil {
			return nil, errors.Wrap(ErrTokenValidation, "invalid server URL in token: "+describeURLError(err))
		}
	}
	ensureCorrectBasePath(serverURL, viper.GetString("server.baseUrl"))
	token.ServerUrl = serverURL.String()

	overwriteTokenURL := viper.GetString(ConfigKeyTokenURL)
	if overwriteTokenURL != "" {
		token.TokenUrl = overwriteTokenURL
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
		return errors.Wrap(ErrTokenValidation, describeValidationError(err))
	}
	return nil
}

// The describe* helpers build debug details for wrapping the customer-facing
// sentinels. errors.Cause still yields the sentinel, so normal output stays
// friendly. They only report positions, field names and rules, never
// supplied values, which may contain credentials.

func describeBase64Error(err error) string {
	var corrupt base64.CorruptInputError
	if errors.As(err, &corrupt) {
		return fmt.Sprintf("decoding token: illegal base64 data at input byte %d", int64(corrupt))
	}
	return "decoding token: invalid base64 data"
}

func describeJSONError(err error) string {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Sprintf("parsing token JSON: syntax error at offset %d", syntaxErr.Offset)
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return fmt.Sprintf("parsing token JSON: field %q must be of type %s", typeErr.Field, typeErr.Type)
	}
	return "parsing token JSON: invalid JSON"
}

// describeURLError classifies url.Parse failures. The parser's messages quote
// the supplied URL or parts of it, so only a fixed category is returned.
func describeURLError(err error) string {
	var escapeErr url.EscapeError
	if errors.As(err, &escapeErr) {
		return "invalid URL escape"
	}
	var hostErr url.InvalidHostError
	if errors.As(err, &hostErr) {
		return "invalid host"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		reason := urlErr.Err.Error()
		switch {
		case strings.HasPrefix(reason, "invalid port"):
			return "invalid port"
		case strings.HasPrefix(reason, "missing protocol scheme"):
			return "missing protocol scheme"
		}
	}
	return "malformed URL"
}

func describeValidationError(err error) string {
	var fieldErrs validator.ValidationErrors
	if !errors.As(err, &fieldErrs) {
		return "validating token: unexpected validation failure"
	}
	failures := make([]string, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		failures = append(failures, fmt.Sprintf("%s failed rule %q", fe.Namespace(), fe.Tag()))
	}
	return "validating token: " + strings.Join(failures, ", ")
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

	return context.WithValue(ctx, "token", token)
}

func FromContext(ctx context.Context) (*Token, bool) {
	token, ok := ctx.Value("token").(*Token)
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
	if expectedPath == "" || expectedPath == "/" {
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
