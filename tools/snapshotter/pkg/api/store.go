// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"go.uber.org/zap"
)

// GET /snapshots?id={snapshotID}
func (a *API) GetSnapshot(ctx *fiber.Ctx) error {
	id := ctx.Query("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "id query parameter is required")
	}
	version := ctx.QueryInt("version", 0)

	snaps := &snapshot.SnapshotList{}

	if version >= 0 {
		zap.L().Info("getting snapshot version", zap.String("id", id), zap.Int("version", version))
		snap := snaps.New()
		err := a.store.GetVersion(ctx.Context(), id, version, snap)
		if err != nil {
			return err
		}
		snaps.Add(snap)
	} else {
		zap.L().Info("getting all snapshot versions", zap.String("id", id))
		err := a.store.GetAll(ctx.Context(), id, snaps)
		if err != nil {
			return err
		}
	}
	return ctx.JSON(snaps)
}

// DELETE /snapshots?id={snapshotID}
func (a *API) DeleteSnapshot(ctx *fiber.Ctx) error {
	id := ctx.Query("id")
	zap.L().Info("deleting snapshot", zap.String("id", id))
	err := a.store.Delete(ctx.Context(), id)
	if err != nil {
		return err
	}
	return ctx.SendStatus(fiber.StatusNoContent)
}
