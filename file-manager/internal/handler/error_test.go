// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"

	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

func TestErrorHandler(t *testing.T) {
	const fileId = "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60"

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", backend.ErrFileNotFound(fileId), fiber.StatusNotFound},
		{"invalid file id", backend.ErrInvalidFileId(fileId), fiber.StatusBadRequest},
		{"file exists", backend.ErrFileExists(fileId), fiber.StatusConflict},
		{"too many requests", backend.ErrTooManyRequests(fileId), fiber.StatusTooManyRequests},
		{"invalid checksum", backend.ErrInvalidChecksum(fileId, "a", "b"), fiber.StatusUnprocessableEntity},
		{"invalid content type", backend.ErrInvalidContentType(fileId, "a", "b"), fiber.StatusUnprocessableEntity},
		{"client initialization", backend.ErrClientInitialization("boom"), fiber.StatusServiceUnavailable},
		{"upload failed", backend.ErrUploadFailed(fileId, "boom"), fiber.StatusInternalServerError},
		{"download failed", backend.ErrDownloadFailed(fileId, "boom"), fiber.StatusInternalServerError},
		{"unknown backend type", backend.NewBackendError(fileId, errors.New("boom"), "Unknown"), fiber.StatusInternalServerError},
		{"wrapped backend error", errors.Wrap(backend.ErrFileNotFound(fileId), "context"), fiber.StatusNotFound},
		{"non-backend error", errors.New("boom"), fiber.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
			app.Get("/", func(c *fiber.Ctx) error { return tt.err })

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", http.NoBody))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}
