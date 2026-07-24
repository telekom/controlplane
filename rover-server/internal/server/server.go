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
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
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
	ResetRoverSecret(ctx context.Context, resourceId string) (api.RoverSecretRotationAcceptedResponse, error)
	GetSecretRotationStatus(ctx context.Context, resourceId string) (api.RoverSecretRotationStatusResponse, error)
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

type ApiRoadmapController interface {
	Create(ctx context.Context, req api.ApiRoadmapCreateRequest) (api.ApiRoadmapResponse, error)
	Get(ctx context.Context, resourceId string) (api.ApiRoadmapResponse, error)
	GetAll(ctx context.Context, params api.GetAllApiRoadmapsParams) (*api.ApiRoadmapListResponse, error)
	Update(ctx context.Context, resourceId string, req api.ApiRoadmapUpdateRequest) (api.ApiRoadmapResponse, error)
	Delete(ctx context.Context, resourceId string) error
	GetStatus(ctx context.Context, resourceId string) (api.ResourceStatusResponse, error)
}

type ApiChangelogController interface {
	Create(ctx context.Context, req api.ApiChangelogCreateRequest) (api.ApiChangelogResponse, error)
	Get(ctx context.Context, resourceId string) (api.ApiChangelogResponse, error)
	GetAll(ctx context.Context, params api.GetAllApiChangelogsParams) (*api.ApiChangelogListResponse, error)
	Update(ctx context.Context, resourceId string, req api.ApiChangelogUpdateRequest) (api.ApiChangelogResponse, error)
	Delete(ctx context.Context, resourceId string) error
	GetStatus(ctx context.Context, resourceId string) (api.ResourceStatusResponse, error)
}

type McpSpecificationController interface {
	Create(ctx context.Context, req api.McpSpecificationCreateRequest) (api.McpSpecificationResponse, error)
	Get(ctx context.Context, resourceId string) (api.McpSpecificationResponse, error)
	GetAll(ctx context.Context, params api.GetAllMcpSpecificationsParams) (*api.McpSpecificationListResponse, error)
	Update(ctx context.Context, resourceId string, req api.McpSpecificationUpdateRequest) (api.McpSpecificationResponse, error)
	Delete(ctx context.Context, resourceId string) error
	GetStatus(ctx context.Context, resourceId string) (api.ResourceStatusResponse, error)
}

// SecurityTemplates are the rover-specific check-access comparison templates.
// They are exported so cmd/main.go can inject them into the JWT SecurityOpts
// when building each listener's security family.
var SecurityTemplates = map[security.ClientType]security.ComparisonTemplates{
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
	Roadmaps            ApiRoadmapController
	EventSpecifications EventSpecificationController
	ApiChangelogs       ApiChangelogController
	McpSpecifications   McpSpecificationController
}

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

	router.Use(openapiValidator)

	s.Log.Info("Registering apispecifications routes")

	router.Add(fiber.MethodGet, "/apispecifications", cserver.Guarded(guard, s.GetAllApiSpecifications)...)
	router.Add(fiber.MethodPost, "/apispecifications", cserver.Guarded(guard, s.CreateApiSpecification)...)
	router.Add(fiber.MethodGet, "/apispecifications/:resourceId/status", cserver.Guarded(guard, s.GetApiSpecificationStatus)...)

	router.Add(fiber.MethodGet, "/apispecifications/:resourceId", cserver.Guarded(guard, s.GetApiSpecifications)...)
	router.Add(fiber.MethodPut, "/apispecifications/:resourceId", cserver.Guarded(guard, s.UpdateApiSpecification)...)
	router.Add(fiber.MethodDelete, "/apispecifications/:resourceId", cserver.Guarded(guard, s.DeleteApiSpecification)...)

	s.Log.Info("Registering rovers routes")

	router.Add(fiber.MethodGet, "/rovers", cserver.Guarded(guard, s.GetAllRovers)...)
	router.Add(fiber.MethodPost, "/rovers", cserver.Guarded(guard, s.CreateRover)...)
	router.Add(fiber.MethodGet, "/rovers/info", cserver.Guarded(guard, s.GetManyApplicationInfo)...)

	router.Add(fiber.MethodGet, "/rovers/:resourceId/status", cserver.Guarded(guard, s.GetRoverStatus)...)
	router.Add(fiber.MethodGet, "/rovers/:resourceId/info", cserver.Guarded(guard, s.GetApplicationInfo)...)
	router.Add(fiber.MethodPatch, "/rovers/:resourceId/secret", cserver.Guarded(guard, s.ResetRoverSecret)...)
	router.Add(fiber.MethodGet, "/rovers/:resourceId/secret/status", cserver.Guarded(guard, s.GetSecretRotationStatus)...)

	router.Add(fiber.MethodDelete, "/rovers/:resourceId", cserver.Guarded(guard, s.DeleteRover)...)
	router.Add(fiber.MethodGet, "/rovers/:resourceId", cserver.Guarded(guard, s.GetRover)...)
	router.Add(fiber.MethodPut, "/rovers/:resourceId", cserver.Guarded(guard, s.UpdateRover)...)

	s.Log.Info("Registering eventspecifications routes")

	router.Add(fiber.MethodGet, "/eventspecifications", cserver.Guarded(guard, s.GetAllEventSpecifications)...)
	router.Add(fiber.MethodPost, "/eventspecifications", cserver.Guarded(guard, s.CreateEventSpecification)...)
	router.Add(fiber.MethodGet, "/eventspecifications/:resourceId/status", cserver.Guarded(guard, s.GetEventSpecificationStatus)...)

	router.Add(fiber.MethodGet, "/eventspecifications/:resourceId", cserver.Guarded(guard, s.GetEventSpecification)...)
	router.Add(fiber.MethodPut, "/eventspecifications/:resourceId", cserver.Guarded(guard, s.UpdateEventSpecification)...)
	router.Add(fiber.MethodDelete, "/eventspecifications/:resourceId", cserver.Guarded(guard, s.DeleteEventSpecification)...)

	s.Log.Info("Registering apiroadmaps routes")

	router.Add(fiber.MethodGet, "/apiroadmaps", cserver.Guarded(guard, s.GetAllApiRoadmaps)...)
	router.Add(fiber.MethodPost, "/apiroadmaps", cserver.Guarded(guard, s.CreateApiRoadmap)...)
	router.Add(fiber.MethodGet, "/apiroadmaps/:resourceId/status", cserver.Guarded(guard, s.GetApiRoadmapStatus)...)

	router.Add(fiber.MethodGet, "/apiroadmaps/:resourceId", cserver.Guarded(guard, s.GetApiRoadmap)...)
	router.Add(fiber.MethodPut, "/apiroadmaps/:resourceId", cserver.Guarded(guard, s.UpdateApiRoadmap)...)
	router.Add(fiber.MethodDelete, "/apiroadmaps/:resourceId", cserver.Guarded(guard, s.DeleteApiRoadmap)...)

	s.Log.Info("Registering apichangelogs routes")

	router.Add(fiber.MethodGet, "/apichangelogs", cserver.Guarded(guard, s.GetAllApiChangelogs)...)
	router.Add(fiber.MethodPost, "/apichangelogs", cserver.Guarded(guard, s.CreateApiChangelog)...)
	router.Add(fiber.MethodGet, "/apichangelogs/:resourceId/status", cserver.Guarded(guard, s.GetApiChangelogStatus)...)

	router.Add(fiber.MethodGet, "/apichangelogs/:resourceId", cserver.Guarded(guard, s.GetApiChangelog)...)
	router.Add(fiber.MethodPut, "/apichangelogs/:resourceId", cserver.Guarded(guard, s.UpdateApiChangelog)...)
	router.Add(fiber.MethodDelete, "/apichangelogs/:resourceId", cserver.Guarded(guard, s.DeleteApiChangelog)...)

	s.Log.Info("Registering mcpspecifications routes")

	router.Add(fiber.MethodGet, "/mcpspecifications", cserver.Guarded(guard, s.GetAllMcpSpecifications)...)
	router.Add(fiber.MethodPost, "/mcpspecifications", cserver.Guarded(guard, s.CreateMcpSpecification)...)
	router.Add(fiber.MethodGet, "/mcpspecifications/:resourceId/status", cserver.Guarded(guard, s.GetMcpSpecificationStatus)...)

	router.Add(fiber.MethodGet, "/mcpspecifications/:resourceId", cserver.Guarded(guard, s.GetMcpSpecification)...)
	router.Add(fiber.MethodPut, "/mcpspecifications/:resourceId", cserver.Guarded(guard, s.UpdateMcpSpecification)...)
	router.Add(fiber.MethodDelete, "/mcpspecifications/:resourceId", cserver.Guarded(guard, s.DeleteMcpSpecification)...)

}
