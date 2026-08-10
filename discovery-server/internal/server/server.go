// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	requestValidator "github.com/oapi-codegen/fiber-middleware"
	"github.com/pkg/errors"

	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/config"
)

// ApiExposureController defines the read-only operations for ApiExposure resources.
type ApiExposureController interface {
	Get(ctx context.Context, applicationId, apiExposureName string) (api.ApiExposureResponse, error)
	GetAll(ctx context.Context, applicationId string, params api.GetAllApiExposuresParams) (*api.ApiExposureListResponse, error)
	GetStatus(ctx context.Context, applicationId, apiExposureName string) (api.ResourceStatusResponse, error)
	GetSubscriptions(ctx context.Context, applicationId, apiExposureName string, params api.GetAllExposureApiSubscriptionsParams) (*api.ApiSubscriptionListResponse, error)
}

// ApiSubscriptionController defines the read-only operations for ApiSubscription resources.
type ApiSubscriptionController interface {
	Get(ctx context.Context, applicationId, apiSubscriptionName string) (api.ApiSubscriptionResponse, error)
	GetAll(ctx context.Context, applicationId string, params api.GetAllApiSubscriptionsParams) (*api.ApiSubscriptionListResponse, error)
	GetStatus(ctx context.Context, applicationId, apiSubscriptionName string) (api.ResourceStatusResponse, error)
}

// ApplicationController defines the read-only operations for Application resources.
type ApplicationController interface {
	Get(ctx context.Context, applicationId string) (api.ApplicationResponse, error)
	GetAll(ctx context.Context, params api.GetAllApplicationsParams) (*api.ApplicationListResponse, error)
	GetStatus(ctx context.Context, applicationId string) (api.ResourceStatusResponse, error)
}

// EventExposureController defines the read-only operations for EventExposure resources.
type EventExposureController interface {
	Get(ctx context.Context, applicationId, eventExposureName string) (api.EventExposureResponse, error)
	GetAll(ctx context.Context, applicationId string, params api.GetAllEventExposuresParams) (*api.EventExposureListResponse, error)
	GetStatus(ctx context.Context, applicationId, eventExposureName string) (api.ResourceStatusResponse, error)
	GetSubscriptions(ctx context.Context, applicationId, eventExposureName string, params api.GetAllExposureEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error)
}

// EventSubscriptionController defines the read-only operations for EventSubscription resources.
type EventSubscriptionController interface {
	Get(ctx context.Context, applicationId, eventSubscriptionName string) (api.EventSubscriptionResponse, error)
	GetAll(ctx context.Context, applicationId string, params api.GetAllEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error)
	GetStatus(ctx context.Context, applicationId, eventSubscriptionName string) (api.ResourceStatusResponse, error)
}

// EventTypeController defines the read-only operations for EventType resources.
type EventTypeController interface {
	GetActive(ctx context.Context, eventTypeName string) (api.EventTypeResponse, error)
	Get(ctx context.Context, eventTypeId string) (api.EventTypeResponse, error)
	GetAll(ctx context.Context, params api.GetAllEventTypesParams) (*api.EventTypeListResponse, error)
	GetStatus(ctx context.Context, eventTypeId string) (api.ResourceStatusResponse, error)
}

// SecurityTemplates are the discovery-specific check-access comparison
// templates. They are exported so cmd/main.go can inject them into the JWT
// SecurityOpts when building each listener's security family.
var SecurityTemplates = map[security.ClientType]security.ComparisonTemplates{
	security.ClientTypeTeam: {
		ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}--",
		UserInputTemplate: "{{ .B.Environment }}--{{ .P.Applicationid }}",
		MatchType:         security.MatchTypePrefix,
	},
	security.ClientTypeGroup: {
		ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
		UserInputTemplate: "{{ .B.Environment }}--{{ .P.Applicationid }}",
		MatchType:         security.MatchTypePrefix,
	},
	security.ClientTypeAdmin: {
		ExpectedTemplate:  "{{ .B.Environment }}--",
		UserInputTemplate: "{{ .B.Environment }}--{{ .P.Applicationid }}",
		MatchType:         security.MatchTypePrefix,
	},
}

// Server holds all controllers and configuration needed to serve HTTP requests.
type Server struct {
	Config             *config.ServerConfig
	Log                logr.Logger
	ApiExposures       ApiExposureController
	ApiSubscriptions   ApiSubscriptionController
	Applications       ApplicationController
	EventExposures     EventExposureController
	EventSubscriptions EventSubscriptionController
	EventTypes         EventTypeController
}

// RegisterRoutes sets up OpenAPI validation and all route handlers, attaching
// the security family's per-route guard (nil = no per-route guard).
func (s *Server) RegisterRoutes(router fiber.Router, guard fiber.Handler) {
	swagger, err := api.GetSwagger()
	if err != nil {
		panic(errors.Wrap(err, "failed to get swagger"))
	}
	swagger.Servers = nil

	NoopAuthenticator := func(ctx context.Context, ai *openapi3filter.AuthenticationInput) error {
		return nil
	}

	openapiValidator := requestValidator.OapiRequestValidatorWithOptions(swagger, &requestValidator.Options{
		Options: openapi3filter.Options{
			SkipSettingDefaults: false,
			AuthenticationFunc:  NoopAuthenticator,
		},
	})

	// Deprecated write endpoints must be registered before the OpenAPI validator
	// because the spec does not define these methods — the validator would reject them first.
	s.registerDeprecatedRoutes(router, guard)

	router.Use(openapiValidator)

	// Application routes (read-only)
	s.Log.Info("Registering application routes")
	router.Add(fiber.MethodGet, "/applications", cserver.Guarded(guard, s.GetAllApplications)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId", cserver.Guarded(guard, s.GetApplication)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/status", cserver.Guarded(guard, s.GetApplicationStatus)...)

	// ApiExposure routes (read-only)
	s.Log.Info("Registering apiexposure routes")
	router.Add(fiber.MethodGet, "/applications/:applicationId/apiexposures", cserver.Guarded(guard, s.GetAllApiExposures)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/apiexposures/:apiExposureName", cserver.Guarded(guard, s.GetApiExposure)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/apiexposures/:apiExposureName/status", cserver.Guarded(guard, s.GetApiExposureStatus)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/apiexposures/:apiExposureName/apisubscriptions", cserver.Guarded(guard, s.GetApiExposureSubscriptions)...)

	// ApiSubscription routes (read-only)
	s.Log.Info("Registering apisubscription routes")
	router.Add(fiber.MethodGet, "/applications/:applicationId/apisubscriptions", cserver.Guarded(guard, s.GetAllApiSubscriptions)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/apisubscriptions/:apiSubscriptionName", cserver.Guarded(guard, s.GetApiSubscription)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/apisubscriptions/:apiSubscriptionName/status", cserver.Guarded(guard, s.GetApiSubscriptionStatus)...)

	// EventExposure routes (read-only)
	s.Log.Info("Registering eventexposure routes")
	router.Add(fiber.MethodGet, "/applications/:applicationId/eventexposures", cserver.Guarded(guard, s.GetAllEventExposures)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/eventexposures/:eventExposureName", cserver.Guarded(guard, s.GetEventExposure)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/eventexposures/:eventExposureName/status", cserver.Guarded(guard, s.GetEventExposureStatus)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/eventexposures/:eventExposureName/eventsubscriptions", cserver.Guarded(guard, s.GetEventExposureSubscriptions)...)

	// EventSubscription routes (read-only)
	s.Log.Info("Registering eventsubscription routes")
	router.Add(fiber.MethodGet, "/applications/:applicationId/eventsubscriptions", cserver.Guarded(guard, s.GetAllEventSubscriptions)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/eventsubscriptions/:eventSubscriptionName", cserver.Guarded(guard, s.GetEventSubscription)...)
	router.Add(fiber.MethodGet, "/applications/:applicationId/eventsubscriptions/:eventSubscriptionName/status", cserver.Guarded(guard, s.GetEventSubscriptionStatus)...)

	// EventType routes (read-only) — intentionally unguarded (public).
	s.Log.Info("Registering eventtype routes")
	router.Add(fiber.MethodGet, "/eventtypes", s.GetAllEventTypes)
	router.Add(fiber.MethodGet, "/eventtypes/:eventTypeId", s.GetEventType)
	router.Add(fiber.MethodGet, "/eventtypes/:eventTypeId/status", s.GetEventTypeStatus)
	router.Add(fiber.MethodGet, "/eventtypes/:eventTypeName/active", s.GetActiveEventType)
}
