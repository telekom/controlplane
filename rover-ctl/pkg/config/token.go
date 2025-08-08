// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

type Token struct {
	Prefix      string `json:"prefix"`
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

	if token.ServerUrl == "" {
		token.ServerUrl = viper.GetString(ConfigKeyServerURL)
	}
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
	return time.UnixMilli(t.GeneratedAt).Format(time.RFC3339)
}

func (t *Token) TimeSinceGenerated() string {
	if t.GeneratedAt == 0 {
		return "unknown"
	}
	delta := time.Since(time.UnixMilli(t.GeneratedAt)).Abs()

	if delta < time.Minute {
		return "just now"
	} else if delta < time.Hour {
		return "less than an hour ago"
	} else if delta < 24*time.Hour {
		return "less than a day ago"
	} else if delta < 7*24*time.Hour {
		return "less than a week ago"
	} else if delta < 30*24*time.Hour {
		return "less than a month ago"
	} else if delta < 365*24*time.Hour {
		return "less than a year ago"
	}
	return "more than a year ago"
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

// String returns the token in a human-readable format
// It will obfuscate sensitive information like ClientSecret
// and return the rest of the token details.
func (t *Token) String() string {
	return "Token{" +
		"Environment: " + t.Environment + ", " +
		"Group: " + t.Group + ", " +
		"Team: " + t.Team + ", " +
		"ClientId: " + t.ClientId + ", " +
		"ClientSecret: [REDACTED], " +
		"TokenUrl: " + t.TokenUrl + ", " +
		"ServerUrl: " + t.ServerUrl + ", " +
		"GeneratedAt: " + t.GeneratedString() +
		"}"

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
