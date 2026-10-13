// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
)

func ReturnWithProblem(ctx *fiber.Ctx, problem problems.Problem, err error) error {
	if problem != nil {
		return ctx.Status(problem.Code()).JSON(problem, "application/problem+json")
	}

	return ReturnWithError(ctx, err)
}

func ReturnWithError(c *fiber.Ctx, err error) error {
	log := logr.FromContextOrDiscard(c.UserContext())
	var p problems.Problem
	if errors.As(err, &p) {
		problemType, fields := problems.Details(p)
		logErrorResponse(log, err, p.Code(), "problemType", problemType, "status", p.Code(), "problemFields", fields)
		return c.Status(p.Code()).JSON(p, "application/problem+json")
	}

	if errors.Is(err, context.DeadlineExceeded) {
		log.Error(err, "Returning error response")
		p = problems.Builder().
			Status(503).
			Type("Timeout").
			Title("Request Timeout").
			Detail("The server timed out waiting for the request").
			Instance(c.OriginalURL()).Build()

		return c.Status(p.Code()).JSON(p, "application/problem+json")
	}

	var fe *fiber.Error
	if errors.As(err, &fe) {
		logErrorResponse(log, err, fe.Code)
		if fe.Code >= 500 {
			p = problems.NewProblemOfError(err)
		} else if fe.Code == 404 {
			p = problems.NotFound()
		} else if fe.Code == 405 {
			p = problems.MethodNotAllowed(c.Method())
		} else {
			p = problems.BadRequest(fe.Message)
		}
		return c.Status(fe.Code).JSON(p, "application/problem+json")
	}

	problem := problems.NewProblemOfError(err)
	logErrorResponse(log, err, problem.Code())
	return c.Status(problem.Code()).JSON(problem, "application/problem+json")
}

// logErrorResponse logs 5xx responses at error level with the original error
// and client errors at info level with only the error message, so expected
// client errors do not emit stack traces.
func logErrorResponse(log logr.Logger, err error, status int, keysAndValues ...any) {
	if status >= 500 {
		log.Error(err, "Returning error response", keysAndValues...)
		return
	}
	log.Info("Returning error response", append([]any{"error", err.Error()}, keysAndValues...)...)
}
