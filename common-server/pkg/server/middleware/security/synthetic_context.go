// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
)

// NewSyntheticAdminBusinessContext builds a BusinessContext for a
// pre-authenticated, fully-trusted internal caller (a validated ServiceAccount
// on the internal port). It mirrors what the JWT business-context + checkAccess
// path sets, so downstream handlers that read FromContext / PrefixFromContext
// keep working — but authorization is deliberately skipped: the caller has
// already been validated and allow-listed by the Kubernetes-authz middleware.
//
// Behaviour is fixed by the settled design and takes no options:
//   - Environment comes from the mandatory X-Environment header.
//   - ClientType is always admin; AccessType is always read-write ("all").
//   - The store prefix is "<env>--" (the admin template).
//
// A missing or empty X-Environment is rejected with 400 — a blank env would
// yield the prefix "--", which store filtering treats as *all* environments.
func NewSyntheticAdminBusinessContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		env := c.Get("X-Environment")
		if env == "" {
			p := problems.BadRequest("Missing or empty X-Environment header")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		}

		bCtx := &BusinessContext{
			Environment: env,
			ClientType:  ClientTypeAdmin,
			AccessType:  AccessTypeReadWrite,
		}
		prefix := env + "--"

		// Write both the fiber Local (read by checkAccess-style helpers) and the
		// UserContext value (read by FromContext / PrefixFromContext), using the
		// same keys the JWT + checkAccess path uses.
		c.Locals("businessContext", bCtx)
		c.Locals("prefix", prefix)

		ctx := c.UserContext()
		ctx = context.WithValue(ctx, prefixKey, prefix)
		ctx = ToContext(ctx, bCtx)
		c.SetUserContext(ctx)

		return c.Next()
	}
}
