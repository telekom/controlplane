// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"errors"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

// ErrorHandler handles API errors and returns them in a standardized format
func ErrorHandler(c *fiber.Ctx, err error) error {
	log := logr.FromContextOrDiscard(c.UserContext())
	log.Info("Error handler", "error", err.Error())

	// Handle specific backend errors
	var backendErr *backend.BackendError
	if errors.As(err, &backendErr) {
		switch backendErr.Type {
		case backend.TypeErrNotFound:
			return c.Status(fiber.StatusNotFound).JSON(map[string]interface{}{
				"type":   "NotFound",
				"status": fiber.StatusNotFound,
				"title":  "Not Found",
				"detail": backendErr.Error(),
			})
		case backend.TypeErrInvalidFileId:
			return c.Status(fiber.StatusBadRequest).JSON(map[string]interface{}{
				"type":   "BadRequest",
				"status": fiber.StatusBadRequest,
				"title":  "Invalid File ID",
				"detail": backendErr.Error(),
			})
		case backend.TypeErrFileExists:
			return c.Status(fiber.StatusConflict).JSON(map[string]interface{}{
				"type":   "Conflict",
				"status": fiber.StatusConflict,
				"title":  "File Already Exists",
				"detail": backendErr.Error(),
			})
		case backend.TypeErrTooManyRequests:
			return c.Status(fiber.StatusTooManyRequests).JSON(map[string]interface{}{
				"type":   "TooManyRequests",
				"status": fiber.StatusTooManyRequests,
				"title":  "Too Many Requests",
				"detail": backendErr.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(map[string]interface{}{
				"type":   "InternalServerError",
				"status": fiber.StatusInternalServerError,
				"title":  "Internal Server Error",
				"detail": backendErr.Error(),
			})
		}
	}

	// For non-backend errors, use the common server error handler
	return server.ReturnWithError(c, err)
}
