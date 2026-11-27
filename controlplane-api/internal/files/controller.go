// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/file-manager/api/constants"
)

const routePath = "/files/v1/:fileId"

// Controller proxies file downloads from the internal file-manager.
type Controller struct {
	downloader api.DownloadApi
}

func NewController(downloader api.DownloadApi) *Controller {
	return &Controller{downloader: downloader}
}

// RegisterRoutes registers GET /files/v1/:fileId.
//
// The route is public by design: the controlplane-api is read-only and serves
// public data, so the guard chain is intentionally not applied. The internal
// listener's family still applies its own middleware. Only GET is registered
// (router.Get would also register HEAD, which would trigger an upstream
// download); other methods are answered by the server with a 405 problem.
func (c *Controller) RegisterRoutes(router fiber.Router, _ []fiber.Handler) {
	router.Add(fiber.MethodGet, routePath, c.getFile)
}

func (c *Controller) getFile(ctx *fiber.Ctx) error {
	id := ctx.Params("fileId")
	if err := ValidateFileID(id); err != nil {
		return cserver.ReturnWithProblem(ctx, problems.BadRequest("invalid file id"), nil)
	}

	log := logr.FromContextOrDiscard(ctx.UserContext())

	var buf bytes.Buffer
	res, err := c.downloader.DownloadFile(ctx.UserContext(), id, &buf)
	if errors.Is(err, api.ErrNotFound) {
		return cserver.ReturnWithProblem(ctx, problems.NotFound("file "+id), nil)
	}
	if err != nil {
		log.Error(err, "Failed to download file from file-manager", "fileId", id)
		return cserver.ReturnWithProblem(ctx, problems.Builder().
			Status(http.StatusBadGateway).
			Type("BadGateway").
			Title("Bad Gateway").
			Detail("file could not be retrieved").
			Build(), nil)
	}

	log.V(1).Info("Served file download", "fileId", id, "size", buf.Len())

	contentType := constants.DefaultContentType
	if res != nil && res.ContentType != "" {
		contentType = res.ContentType
	}
	ctx.Set(fiber.HeaderContentType, contentType)
	if res != nil && res.FileHash != "" {
		ctx.Set(fiber.HeaderETag, `"`+res.FileHash+`"`)
	}
	ctx.Set("X-Content-Type-Options", "nosniff")
	ctx.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	return ctx.Status(http.StatusOK).Send(buf.Bytes())
}
