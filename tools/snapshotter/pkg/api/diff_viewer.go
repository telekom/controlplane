// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

// DiffViewerData represents the data passed to the HTML template
type DiffViewerData struct {
	SnapshotA       string
	SnapshotB       string
	SnapshotAData   template.JS
	SnapshotBData   template.JS
	Changed         bool
	NumberOfChanges int
}

// DiffViewer renders an HTML diff viewer using Monaco Editor
func (a *API) DiffViewer(ctx *fiber.Ctx) error {
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

	zap.L().Info("comparing snapshots for diff viewer", zap.String("a", snap.ID()), zap.String("b", otherSnap.ID()))
	result := diffmatcher.Compare(snap, otherSnap)

	// Prepare template data
	var snapAContent, snapBContent = snap.String(), otherSnap.String()

	// Safety escape JSON values
	snapAEscaped := template.JS(strings.ReplaceAll(strings.ReplaceAll(string(snapAContent), "\\", "\\\\"), "`", "\\`"))
	snapBEscaped := template.JS(strings.ReplaceAll(strings.ReplaceAll(string(snapBContent), "\\", "\\\\"), "`", "\\`"))

	data := DiffViewerData{
		SnapshotA:       snapshotA,
		SnapshotB:       snapshotB,
		SnapshotAData:   template.JS("`" + snapAEscaped + "`"),
		SnapshotBData:   template.JS("`" + snapBEscaped + "`"),
		Changed:         result.Changed,
		NumberOfChanges: result.NumberOfChanges,
	}

	// Parse and execute template
	tmplPath := filepath.Join("pkg", "api", "templates", "diff-viewer.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		zap.L().Error("failed to parse template", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to load template")
	}

	// Set content type to HTML
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)

	// Render template
	return tmpl.Execute(ctx, data)
}
