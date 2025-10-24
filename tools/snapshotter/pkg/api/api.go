// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

type API struct {
	store     store.SnapshotStore
	instances map[string]*orchestrator.Orchestrator
}

func NewAPI(store store.SnapshotStore, instances map[string]*orchestrator.Orchestrator) *API {
	return &API{
		store:     store,
		instances: instances,
	}
}

func (api *API) RegisterRoutes(app *fiber.App) {
	// app.Get("/snapshots", api.ListSnapshots)
	// app.Get("/snapshots/:id", api.GetSnapshot)
	// app.Delete("/snapshots/:id", api.DeleteSnapshot)

	app.Post("/snapshots", api.TakeSnapshot)
	app.Get("/compare", api.CompareSnapshots)
}
