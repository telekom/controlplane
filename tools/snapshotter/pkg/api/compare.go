// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

// /compare?a={snapshotA}&b={snapshotB}
func (a *API) CompareSnapshots(ctx *fiber.Ctx) error {
	snapshotA := ctx.Query("a")
	snapshotB := ctx.Query("b")

	if snapshotA == "" || snapshotB == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Both a and b query parameters are required")
	}

	snapshot, err := a.store.GetVersion(ctx.Context(), snapshotA, 0) // latest
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}

	snapshotOther, err := a.store.GetVersion(ctx.Context(), snapshotB, 0) // latest
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}

	result := diffmatcher.Compare(&snapshot, &snapshotOther)
	if result.Changed {
		return ctx.JSON(map[string]any{
			"changed":           true,
			"number_of_changes": result.NumberOfChanges,
			"text":              result.Text,
			"a":                 snapshot.String(),
			"b":                 snapshotOther.String(),
		})
	} else {
		return ctx.Status(fiber.StatusOK).JSON(map[string]any{
			"changed": false,
		})
	}
}
