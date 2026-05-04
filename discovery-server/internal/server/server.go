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

var securityTemplates = map[security.ClientType]security.ComparisonTemplates{
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

// RegisterRoutes sets up security middleware, OpenAPI validation, and all route handlers.
func (s *Server) RegisterRoutes(router fiber.Router) {
	checkAccess := security.ConfigureSecurity(router, security.SecurityOpts{
		Enabled: s.Config.Security.Enabled,
		Log:     s.Log,
		JWTOpts: []security.Option[*security.JWTOpts]{
			security.WithLmsCheck(s.Config.Security.LMS.BasePath),
			security.WithTrustedIssuers(s.Config.Security.TrustedIssuers),
		},
		BusinessContextOpts: []security.Option[*security.BusinessContextOpts]{
			security.WithDefaultScope(s.Config.Security.DefaultScope),
			security.WithLog(s.Log),
			security.WithScopePrefix(s.Config.Security.ScopePrefix),
		},
		CheckAccessOpts: []security.Option[*security.CheckAccessOpts]{
			security.WithPathParamKey("applicationId"),
			security.WithTemplates(securityTemplates),
		},
	})

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
	s.registerDeprecatedRoutes(router, checkAccess)

	router.Use(openapiValidator)

	// Application routes (read-only)
	s.Log.Info("Registering application routes")
	router.Get("/applications", checkAccess, s.GetAllApplications)
	router.Get("/applications/:applicationId", checkAccess, s.GetApplication)
	router.Get("/applications/:applicationId/status", checkAccess, s.GetApplicationStatus)

	// ApiExposure routes (read-only)
	s.Log.Info("Registering apiexposure routes")
	router.Get("/applications/:applicationId/apiexposures", checkAccess, s.GetAllApiExposures)
	router.Get("/applications/:applicationId/apiexposures/:apiExposureName", checkAccess, s.GetApiExposure)
	router.Get("/applications/:applicationId/apiexposures/:apiExposureName/status", checkAccess, s.GetApiExposureStatus)
	router.Get("/applications/:applicationId/apiexposures/:apiExposureName/apisubscriptions", checkAccess, s.GetApiExposureSubscriptions)

	// ApiSubscription routes (read-only)
	s.Log.Info("Registering apisubscription routes")
	router.Get("/applications/:applicationId/apisubscriptions", checkAccess, s.GetAllApiSubscriptions)
	router.Get("/applications/:applicationId/apisubscriptions/:apiSubscriptionName", checkAccess, s.GetApiSubscription)
	router.Get("/applications/:applicationId/apisubscriptions/:apiSubscriptionName/status", checkAccess, s.GetApiSubscriptionStatus)

	// EventExposure routes (read-only)
	s.Log.Info("Registering eventexposure routes")
	router.Get("/applications/:applicationId/eventexposures", checkAccess, s.GetAllEventExposures)
	router.Get("/applications/:applicationId/eventexposures/:eventExposureName", checkAccess, s.GetEventExposure)
	router.Get("/applications/:applicationId/eventexposures/:eventExposureName/status", checkAccess, s.GetEventExposureStatus)
	router.Get("/applications/:applicationId/eventexposures/:eventExposureName/eventsubscriptions", checkAccess, s.GetEventExposureSubscriptions)

	// EventSubscription routes (read-only)
	s.Log.Info("Registering eventsubscription routes")
	router.Get("/applications/:applicationId/eventsubscriptions", checkAccess, s.GetAllEventSubscriptions)
	router.Get("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName", checkAccess, s.GetEventSubscription)
	router.Get("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName/status", checkAccess, s.GetEventSubscriptionStatus)

	// EventType routes (read-only)
	s.Log.Info("Registering eventtype routes")
	router.Get("/eventtypes", s.GetAllEventTypes)
	router.Get("/eventtypes/:eventTypeId", s.GetEventType)
	router.Get("/eventtypes/:eventTypeId/status", s.GetEventTypeStatus)
	router.Get("/eventtypes/:eventTypeName/active", s.GetActiveEventType)
}
