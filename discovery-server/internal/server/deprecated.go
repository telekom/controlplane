// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
)

// registerDeprecatedRoutes registers all deprecated write endpoints.
// They return HTTP 410 Gone with an RFC 7807 Problem body directing users to the Rover API.
func (s *Server) registerDeprecatedRoutes(router fiber.Router, checkAccess fiber.Handler) {
	s.Log.Info("Registering deprecated write routes (return 410 Gone)")

	deprecatedHandler := func(c *fiber.Ctx) error {
		problem := problems.Builder().
			Status(http.StatusGone).
			Title("Gone").
			Detail("This endpoint is deprecated. Use the Rover API for write operations.").
			Build()
		return cserver.ReturnWithProblem(c, problem, nil)
	}

	// Application write endpoints
	router.Post("/applications", checkAccess, deprecatedHandler)
	router.Put("/applications/:applicationId", checkAccess, deprecatedHandler)
	router.Delete("/applications/:applicationId", checkAccess, deprecatedHandler)

	// ApiExposure write endpoints
	router.Post("/applications/:applicationId/apiexposures", checkAccess, deprecatedHandler)
	router.Put("/applications/:applicationId/apiexposures/:apiExposureName", checkAccess, deprecatedHandler)
	router.Delete("/applications/:applicationId/apiexposures/:apiExposureName", checkAccess, deprecatedHandler)

	// ApiSubscription write endpoints
	router.Post("/applications/:applicationId/apisubscriptions", checkAccess, deprecatedHandler)
	router.Put("/applications/:applicationId/apisubscriptions/:apiSubscriptionName", checkAccess, deprecatedHandler)
	router.Delete("/applications/:applicationId/apisubscriptions/:apiSubscriptionName", checkAccess, deprecatedHandler)

	// ApiSubscription approve endpoint
	router.Post("/applications/:applicationId/apisubscriptions/:apiSubscriptionName/approve", checkAccess, deprecatedHandler)

	// EventType write endpoints
	router.Post("/eventtypes", checkAccess, deprecatedHandler)

	// EventExposure write endpoints
	router.Post("/applications/:applicationId/eventexposures", checkAccess, deprecatedHandler)
	router.Put("/applications/:applicationId/eventexposures/:eventExposureName", checkAccess, deprecatedHandler)
	router.Delete("/applications/:applicationId/eventexposures/:eventExposureName", checkAccess, deprecatedHandler)

	// EventSubscription write endpoints
	router.Post("/applications/:applicationId/eventsubscriptions", checkAccess, deprecatedHandler)
	router.Put("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName", checkAccess, deprecatedHandler)
	router.Delete("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName", checkAccess, deprecatedHandler)
}
