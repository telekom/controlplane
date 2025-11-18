// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

type API struct {
	store     store.SnapshotStore[*snapshot.Snapshot]
	instances map[string]*orchestrator.Orchestrator
}

func NewAPI(store store.SnapshotStore[*snapshot.Snapshot], instances map[string]*orchestrator.Orchestrator) *API {
	return &API{
		store:     store,
		instances: instances,
	}
}

func (api *API) RegisterRoutes(app *fiber.App) {
	app.Get("/snapshots", api.GetSnapshot)
	app.Delete("/snapshots", api.DeleteSnapshot)

	app.Post("/snapshots", api.TakeSnapshot)
	app.Get("/compare", api.CompareSnapshots)
	app.Get("/diff-viewer", api.DiffViewer)
}
