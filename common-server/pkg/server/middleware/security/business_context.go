// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/util"
)

var invalidCtx = problems.Forbidden("Invalid Context", "Invalid authorization context")
var invalidCtxField = func(field string) problems.Problem {
	return problems.Forbidden("Invalid Context", fmt.Sprintf("Missing field '%s'", field))
}

type BusinessContextOpts struct {
	Log            logr.Logger
	ScopePrefix    string
	DefaultScope   string
	ValuesDecoders map[string]ValueDecoder
}

type BusinessContext struct {
	Environment string
	Group       string
	Team        string
	ClientType  ClientType
	AccessType  AccessType
}

func WithLog(log logr.Logger) Option[*BusinessContextOpts] {
	return func(o *BusinessContextOpts) {
		o.Log = log
	}
}

func WithScopePrefix(prefix string) Option[*BusinessContextOpts] {
	return func(o *BusinessContextOpts) {
		o.ScopePrefix = prefix
	}
}

func WithDefaultScope(scope string) Option[*BusinessContextOpts] {
	return func(o *BusinessContextOpts) {
		o.DefaultScope = scope
	}
}

func WithValueDecoders(decoders map[string]ValueDecoder) Option[*BusinessContextOpts] {
	return func(o *BusinessContextOpts) {
		o.ValuesDecoders = decoders
	}
}

func WithValueDecoder(key string, decoder ValueDecoder) Option[*BusinessContextOpts] {
	return func(o *BusinessContextOpts) {
		o.ValuesDecoders[key] = decoder
	}
}

/*
NewBusinessCtxMiddlewareWithOpts creates a new middleware that extracts the business context from the JWT token
The business context is used to determine the client type and access type
The client type is used to determine which resources this client is allowed to see
The access type is used to determine if the client has read or write access
The actual check if the client is allowed to see a resource is done in `check_access.go`

Example of the JWT token structure:

	{
		"env": "dev",
		"clientId": "group--team--user",
		"scopes": "group:team:read group:team:write",

		# These are used by the JWT middleware
		"operation": "GET",
		"requestPath": "/api/v1/resource",
	}
*/
func NewBusinessCtxMiddlewareWithOpts(opts ...Option[*BusinessContextOpts]) fiber.Handler {
	mwOpts := BusinessContextOpts{
		Log:            logr.Discard(),
		ScopePrefix:    "",
		DefaultScope:   "",
		ValuesDecoders: defaultValuesDecoders,
	}

	for _, o := range opts {
		o(&mwOpts)
	}

	return NewBusinessCtxMiddleware(mwOpts)
}

// NewBusinessCtxMiddleware is a convencience function. See NewBusinessCtxMiddlewareWithOpts
func NewBusinessCtxMiddleware(mwOpts BusinessContextOpts) fiber.Handler {
	defaultScopes := []string{mwOpts.DefaultScope}
	scopePrefix := strings.Trim(mwOpts.ScopePrefix, ":")

	return func(c *fiber.Ctx) error {
		user, ok := util.NotNilOfType[*jwt.Token](c.Locals("user"))
		if !ok {
			return c.Status(invalidCtx.Code()).JSON(invalidCtx, "application/problem+json")
		}

		claims, ok := util.NotNilOfType[jwt.MapClaims](user.Claims)
		if !ok {
			return c.Status(invalidCtx.Code()).JSON(invalidCtx, "application/problem+json")
		}

		env, pErr := DecodeValue(mwOpts.ValuesDecoders, claims, "env")
		if pErr != nil {
			return c.Status(pErr.Code()).JSON(pErr, "application/problem+json")
		}
		group, pErr := DecodeValue(mwOpts.ValuesDecoders, claims, "group")
		if pErr != nil {
			return c.Status(pErr.Code()).JSON(pErr, "application/problem+json")
		}
		team, pErr := DecodeValue(mwOpts.ValuesDecoders, claims, "team")
		if pErr != nil {
			return c.Status(pErr.Code()).JSON(pErr, "application/problem+json")
		}

		var scopes []string
		if scopesClaim, pErr := DecodeValue(mwOpts.ValuesDecoders, claims, "scopes"); pErr == nil {
			scopes = strings.Split(scopesClaim, " ")

		} else if mwOpts.DefaultScope != "" {
			scopes = defaultScopes

		} else {
			return c.Status(invalidCtx.Code()).JSON(invalidCtxField("scopes"), "application/problem+json")
		}

		clientType, err := DetermineClientType(scopes, scopePrefix)
		if err != nil {
			return c.Status(http.StatusForbidden).JSON(err, "application/problem+json")
		}

		accessType, err := DetermineAccess(scopes, scopePrefix)
		if err != nil {
			return c.Status(http.StatusForbidden).JSON(err, "application/problem+json")
		}

		bCtx := &BusinessContext{
			Environment: env,
			Group:       group,
			Team:        team,
			ClientType:  clientType,
			AccessType:  accessType,
		}
		c.Locals("businessContext", bCtx)

		// prefer the logger from the context if available
		// otherwise use the logger from the options
		ctxLog, err := logr.FromContext(c.UserContext())
		if err != nil {
			ctxLog = mwOpts.Log.WithName("reqctx")
		}
		ctx := logr.NewContext(c.UserContext(), ctxLog)
		ctx = context.WithValue(ctx, businessContextKey, bCtx)
		c.SetUserContext(ctx)

		return c.Next()
	}
}

// ClientType represents the type of the client
// Therefore, it is used to determine which resources this client is allowed to see
type ClientType string

const (
	ClientTypeTeam  ClientType = "team"
	ClientTypeGroup ClientType = "group"
	ClientTypeAdmin ClientType = "admin"
)

// AccessType represents the access type of the client
// Therefore, it is used to determine if the client has read or write access
type AccessType string

func (a AccessType) IsRead() bool {
	return a == AccessTypeReadOnly || a == AccessTypeObfuscated || a == AccessTypeReadWrite
}

func (a AccessType) IsWrite() bool {
	return a == AccessTypeReadWrite
}

func (a AccessType) IsObfuscated() bool {
	return a == AccessTypeObfuscated
}

const (
	AccessTypeReadOnly   AccessType = "read"
	AccessTypeObfuscated AccessType = "obfuscated"
	AccessTypeReadWrite  AccessType = "all"
)

// DetermineAccess determines the access type based on the scopes
// It is expeceted that all relevant scopes follow the pattern `<prefix>:<clientType>:<accessType>`
// Therefore, it is used to determine if the client has read or write access
func DetermineAccess(scopes []string, prefix string) (AccessType, error) {
	for _, scope := range scopes {
		parts := strings.Split(scope, ":")
		if prefix != "" && parts[0] != prefix {
			continue
		}

		switch parts[len(parts)-1] {
		case string(AccessTypeReadOnly):
			return AccessTypeReadOnly, nil
		case string(AccessTypeObfuscated):
			return AccessTypeObfuscated, nil
		case string(AccessTypeReadWrite):
			return AccessTypeReadWrite, nil
		}
	}
	return "", problems.Forbidden("Access denied", "No valid scope found")
}

// DetermineClientType determines the client type based on the scopes
// It is expeceted that all relevant scopes follow the pattern `<prefix>:<clientType>:<accessType>`
// Therefore, it is used to determine which resources this client is allowed to see
// e.g. owned by the team, group or all
func DetermineClientType(scopes []string, prefix string) (ClientType, error) {
	for _, scope := range scopes {
		parts := strings.Split(scope, ":")
		if len(parts) < 2 {
			continue
		}
		if prefix != "" && parts[0] != prefix {
			continue
		}
		switch parts[len(parts)-2] {
		case string(ClientTypeTeam):
			return ClientTypeTeam, nil
		case string(ClientTypeGroup):
			return ClientTypeGroup, nil
		case string(ClientTypeAdmin):
			return ClientTypeAdmin, nil
		}

	}
	return "", problems.Forbidden("Access denied", "No valid scope found")
}

func FromContext(ctx context.Context) (*BusinessContext, bool) {
	bCtx, ok := ctx.Value(businessContextKey).(*BusinessContext)
	return bCtx, ok
}

func ToContext(ctx context.Context, bCtx *BusinessContext) context.Context {
	return context.WithValue(ctx, businessContextKey, bCtx)
}

func IsObfuscated(ctx context.Context) bool {
	bCtx, ok := FromContext(ctx)
	if !ok {
		return false
	}
	return bCtx.AccessType.IsObfuscated()
}
