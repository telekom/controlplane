// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
)

// POST /snapshots?instance={instanceID}
func (a *API) TakeSnapshot(ctx *fiber.Ctx) error {
	instanceID := ctx.Query("instance")
	if instanceID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "instance query parameter is required")
	}
	resourceType := ctx.Query("resourceType")

	limit := ctx.QueryInt("limit", 0)

	o, exists := a.instances[instanceID]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "orchestrator instance not found")
	}

	snaps, err := o.Run(ctx.Context(), orchestrator.RunOptions{
		Limit:        limit,
		ResourceType: resourceType,
	})
	if err != nil {
		return err
	}

	var resp []any
	for _, snap := range snaps {
		resp = append(resp, map[string]any{
			"id":   snap.ID,
			"text": snap.String(),
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(resp)
}
