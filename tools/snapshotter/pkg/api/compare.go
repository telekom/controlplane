// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

// /compare?a={snapshotA}&b={snapshotB}
func (a *API) CompareSnapshots(ctx *fiber.Ctx) error {
	snapshotA := ctx.Query("a")
	snapshotB := ctx.Query("b")

	if snapshotA == "" || snapshotB == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Both a and b query parameters are required")
	}

	snap := &snapshot.Snapshot{}
	err := a.store.GetVersion(ctx.Context(), snapshotA, 0, snap) // latest
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		zap.L().Info("snapshot not found", zap.String("id", snapshotA))
	}

	otherSnap := &snapshot.Snapshot{}
	err = a.store.GetVersion(ctx.Context(), snapshotB, 0, otherSnap) // latest
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		zap.L().Info("snapshot not found", zap.String("id", snapshotB))
	}

	zap.L().Info("comparing snapshots", zap.String("a", snap.ID()), zap.String("b", otherSnap.ID()))
	result := diffmatcher.Compare(snap, otherSnap)
	diffViwerUrl, err := url.Parse(ctx.BaseURL())
	if err != nil {
		return err
	}
	diffViwerUrl.Path += "/diff-viewer"
	query := diffViwerUrl.Query()
	query.Set("a", snapshotA)
	query.Set("b", snapshotB)
	diffViwerUrl.RawQuery = query.Encode()
	if result.Changed {
		return ctx.JSON(map[string]any{
			"changed":           true,
			"number_of_changes": result.NumberOfChanges,
			"text":              result.Text,
			"a":                 snap.String(),
			"b":                 otherSnap.String(),
			"diff_viewer_url":   diffViwerUrl.String(),
		})
	} else {
		return ctx.Status(fiber.StatusOK).JSON(map[string]any{
			"changed":         false,
			"diff_viewer_url": diffViwerUrl.String(),
		})
	}
}
