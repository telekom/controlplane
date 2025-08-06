// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/getkin/kin-openapi/openapi3filter"
	requestValidator "github.com/oapi-codegen/fiber-middleware"
	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/config"
)

type RoverController interface {
	Create(ctx context.Context, req api.RoverCreateRequest) (api.RoverResponse, error)
	Get(ctx context.Context, resourceId string) (api.RoverResponse, error)
	GetAll(ctx context.Context, params api.GetAllRoversParams) (*api.RoverListResponse, error)
	Update(ctx context.Context, resourceId string, req api.RoverUpdateRequest) (api.RoverResponse, error)
	Delete(ctx context.Context, resourceId string) error
	GetStatus(ctx context.Context, resourceId string) (api.ResourceStatusResponse, error)
	GetApplicationInfo(ctx context.Context, resourceId string, params api.GetApplicationInfoParams) (api.RoverInfoResponse, error)
	GetApplicationsInfo(ctx context.Context, params api.GetApplicationsInfoParams) (api.RoverInfoResponse, error)
	ResetRoverSecret(ctx context.Context, resourceId string) (api.RoverSecretResponse, error)
}

type ApiSpecificationController interface {
	Create(ctx context.Context, req api.ApiSpecificationCreateRequest) (api.ApiSpecificationResponse, error)
	Get(ctx context.Context, resourceId string) (api.ApiSpecificationResponse, error)
	GetAll(ctx context.Context, params api.GetAllApiSpecificationsParams) (*api.ApiSpecificationListResponse, error)
	Update(ctx context.Context, resourceId string, req api.ApiSpecificationUpdateRequest) (api.ApiSpecificationResponse, error)
	Delete(ctx context.Context, resourceId string) error
	GetStatus(ctx context.Context, resourceId string) (api.ResourceStatusResponse, error)
}

type EventSpecificationController interface {
	Create(ctx context.Context, req api.EventSpecificationCreateRequest) (api.EventSpecificationResponse, error)
	Get(ctx context.Context, resourceId string) (api.EventSpecificationResponse, error)
	GetAll(ctx context.Context, params api.GetAllEventSpecificationsParams) (*api.EventSpecificationListResponse, error)
	Update(ctx context.Context, resourceId string, req api.EventSpecificationUpdateRequest) (api.EventSpecificationResponse, error)
	Delete(ctx context.Context, resourceId string) error
	GetStatus(ctx context.Context, resourceId string) (api.ResourceStatusResponse, error)
}

var securityTemplates = map[security.ClientType]security.ComparisonTemplates{
	security.ClientTypeTeam: {
		ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}--",
		UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
		MatchType:         security.MatchTypePrefix,
	},
	security.ClientTypeGroup: {
		ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
		UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
		MatchType:         security.MatchTypePrefix,
	},
	security.ClientTypeAdmin: {
		ExpectedTemplate:  "{{ .B.Environment }}--",
		UserInputTemplate: "{{ .B.Environment }}--{{ .P.Resourceid }}",
		MatchType:         security.MatchTypePrefix,
	},
}

type Server struct {
	Config              *config.ServerConfig
	Log                 logr.Logger
	ApiSpecifications   ApiSpecificationController
	Rovers              RoverController
	EventSpecifications EventSpecificationController
}

func (s *Server) RegisterRoutes(router fiber.Router) {
	checkAccess := security.ConfigureSecurity(router, security.SecurityOpts{
		Enabled: true,
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
			security.WithPathParamKey("resourceId"),
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
	router.Use(requestValidator.OapiRequestValidatorWithOptions(swagger, &requestValidator.Options{
		Options: openapi3filter.Options{
			SkipSettingDefaults: false,
			AuthenticationFunc:  NoopAuthenticator,
		},
	}))

	s.Log.Info("Registering apispecifications routes")

	router.Get("/apispecifications", checkAccess, s.GetAllApiSpecifications)
	router.Post("/apispecifications", checkAccess, s.CreateApiSpecification)
	router.Get("/apispecifications/:resourceId/status", checkAccess, s.GetApiSpecificationStatus)

	router.Get("/apispecifications/:resourceId", checkAccess, s.GetApiSpecifications)
	router.Put("/apispecifications/:resourceId", checkAccess, s.UpdateApiSpecification)
	router.Delete("/apispecifications/:resourceId", checkAccess, s.DeleteApiSpecification)

	s.Log.Info("Registering rovers routes")

	router.Get("/rovers", checkAccess, s.GetAllRovers)
	router.Post("/rovers", checkAccess, s.CreateRover)
	router.Get("/rovers/info", checkAccess, s.GetManyApplicationInfo)

	router.Get("/rovers/:resourceId/status", checkAccess, s.GetRoverStatus)
	router.Get("/rovers/:resourceId/info", checkAccess, s.GetApplicationInfo)
	router.Patch("/rovers/:resourceId/secret", checkAccess, s.ResetRoverSecret)

	router.Delete("/rovers/:resourceId", checkAccess, s.DeleteRover)
	router.Get("/rovers/:resourceId", checkAccess, s.GetRover)
	router.Put("/rovers/:resourceId", checkAccess, s.UpdateRover)

	s.Log.Info("Registering eventspecifications routes")

	router.Get("/eventspecifications", checkAccess, s.GetAllEventSpecifications)
	router.Post("/eventspecifications", checkAccess, s.CreateEventSpecification)
	router.Get("/eventspecifications/:resourceId/status", checkAccess, s.GetEventSpecificationStatus)

	router.Get("/eventspecifications/:resourceId", checkAccess, s.GetEventSpecification)
	router.Put("/eventspecifications/:resourceId", checkAccess, s.UpdateEventSpecification)
	router.Delete("/eventspecifications/:resourceId", checkAccess, s.DeleteEventSpecification)

}
