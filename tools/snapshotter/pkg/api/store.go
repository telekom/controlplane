// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/gofiber/fiber/v2"

func (a *API) GetSnapshots(ctx *fiber.Ctx) error {
	snapshots, err := a.store.List(ctx.Context())
	if err != nil {
		return err
	}
	return ctx.JSON(snapshots)
}
