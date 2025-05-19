// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"errors"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/secret-manager/internal/api"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

func ErrorHandler(c *fiber.Ctx, err error) error {
	log := logr.FromContextOrDiscard(c.UserContext())
	log.Info("Error handler", "error", err.Error())
	var backendErr *backend.BackendError
	if errors.As(err, &backendErr) {
		switch backendErr.Type {
		case backend.TypeErrNotFound:
			return c.Status(fiber.StatusNotFound).JSON(api.ErrorResponse{
				Status: fiber.StatusNotFound,
				Title:  "Not Found",
				Detail: backendErr.Err.Error(),
			})
		case backend.TypeErrBadChecksum:
			return c.Status(fiber.StatusConflict).JSON(api.ErrorResponse{
				Status: fiber.StatusBadRequest,
				Title:  "Bad Checksum",
				Detail: backendErr.Err.Error(),
			})
		case backend.TypeErrTooManyRequests:
			return c.Status(fiber.StatusTooManyRequests).JSON(api.ErrorResponse{
				Status: fiber.StatusTooManyRequests,
				Title:  "Too Many Requests",
				Detail: backendErr.Err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(api.ErrorResponse{
				Status: fiber.StatusInternalServerError,
				Title:  "Internal Server Error",
				Detail: backendErr.Err.Error(),
			})
		}
	}

	return server.ReturnWithError(c, err)
}
