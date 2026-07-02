// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

type contextKey string

const (
	businessContextKey contextKey = "businessContext"
	prefixKey          contextKey = "prefix"
)

type Option[T any] func(T)

// ModeJWT enables full JWT validation against trusted issuers.
// Requires at least one TrustedIssuer to be configured.
const ModeJWT = "jwt"

// ModeMock enables JWT parsing without signature validation.
// For integration testing with controlled tokens only — never use in production.
const ModeMock = "mock"

type SecurityOpts struct {
	// Mode controls the authentication behaviour.
	// Use the ModeJWT or ModeMock constants.
	Mode string
	Log  logr.Logger

	JWTOpts             []Option[*JWTOpts]
	BusinessContextOpts []Option[*BusinessContextOpts]
	CheckAccessOpts     []Option[*CheckAccessOpts]
}

// ConfigureSecurity configures the security middlewares based on SecurityOpts.Mode.
// It returns the checkAccess middleware to be applied on individual routes.
//
// Mode behaviour:
//   - ModeJWT ("jwt")  — Full JWT validation against trusted issuers. Panics if no trusted issuers are configured.
//   - ModeMock ("mock") — JWT parsed without signature validation. Logs a prominent warning.
//
// Panics on any other Mode value (including empty string).
func ConfigureSecurity(router fiber.Router, opts SecurityOpts) fiber.Handler {
	busCtx := NewBusinessCtxMiddlewareWithOpts(opts.BusinessContextOpts...)
	checkAccess := NewCheckAccessMiddlewareWithOpts(opts.CheckAccessOpts...)

	switch opts.Mode {
	case ModeJWT:
		jwtOpts := &JWTOpts{}
		for _, f := range opts.JWTOpts {
			f(jwtOpts)
		}
		if len(jwtOpts.TrustedIssuers) == 0 {
			panic("security.mode=jwt requires at least one trustedIssuer — configure security.trustedIssuers or set security.mode=mock for integration testing")
		}
		opts.Log.Info("🔒 Security mode: JWT validation enabled")
		router.Use(NewJWTWithOpts(opts.JWTOpts...))
		router.Use(busCtx)
		return checkAccess

	case ModeMock:
		opts.Log.Info("⚠️  Security mode: mock — JWT signatures NOT validated. DO NOT USE IN PRODUCTION.")
		router.Use(mock.NewJWTMock())
		router.Use(busCtx)
		return checkAccess

	default:
		panic(fmt.Sprintf("invalid security.mode: %q (must be one of: mock, jwt)", opts.Mode))
	}
}
