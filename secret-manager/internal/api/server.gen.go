// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package api provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.4.1 DO NOT EDIT.
package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gofiber/fiber/v2"
	"github.com/oapi-codegen/runtime"
)

// ApiProblem Based on https://www.rfc-editor.org/rfc/rfc9457.html
type ApiProblem struct {
	Detail   string  `json:"detail"`
	Instance *string `json:"instance,omitempty"`
	Status   int     `json:"status"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
}

// ListSecretItem defines model for ListSecretItem.
type ListSecretItem struct {
	// Id A reference to a secret
	Id SecretRef `json:"id"`

	// Name The name of the secret
	Name string `json:"name"`
}

// Secret defines model for Secret.
type Secret struct {
	// Id A reference to a secret
	Id SecretRef `json:"id"`

	// Value If empty, a random secret will be generated
	Value string `json:"value"`
}

// SecretRef A reference to a secret
type SecretRef = string

// QuerySecretId defines model for QuerySecretId.
type QuerySecretId = []SecretRef

// SecretId A reference to a secret
type SecretId = SecretRef

// ErrorResponse Based on https://www.rfc-editor.org/rfc/rfc9457.html
type ErrorResponse = ApiProblem

// OnboardingResponse defines model for OnboardingResponse.
type OnboardingResponse struct {
	// Items A list of available secret references
	Items []ListSecretItem `json:"items"`
}

// SecretListReponse defines model for SecretListReponse.
type SecretListReponse struct {
	// Items A list of secrets
	Items []Secret `json:"items"`
}

// SecretResponse defines model for SecretResponse.
type SecretResponse = Secret

// SecretWriteResponse defines model for SecretWriteResponse.
type SecretWriteResponse struct {
	// Id A reference to a secret
	Id SecretRef `json:"id"`
}

// SecretWriteRequest defines model for SecretWriteRequest.
type SecretWriteRequest struct {
	// Value This is the value of the secret.
	// If set to `{{rotate}}`, the secret will be randomly generated
	// and rotated.
	// Otherwise, you can set any string value.
	Value string `json:"value"`
}

// ListSecretsParams defines parameters for ListSecrets.
type ListSecretsParams struct {
	// SecretId The id or reference to a secret
	SecretId *QuerySecretId `form:"secretId,omitempty" json:"secretId,omitempty"`
}

// PutSecretJSONBody defines parameters for PutSecret.
type PutSecretJSONBody struct {
	// Value This is the value of the secret.
	// If set to `{{rotate}}`, the secret will be randomly generated
	// and rotated.
	// Otherwise, you can set any string value.
	Value string `json:"value"`
}

// PutSecretJSONRequestBody defines body for PutSecret for application/json ContentType.
type PutSecretJSONRequestBody PutSecretJSONBody

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Delete an environment
	// (DELETE /v1/onboarding/environments/{envId})
	DeleteEnvironment(c *fiber.Ctx, envId string) error
	// Create or update an environment
	// (PUT /v1/onboarding/environments/{envId})
	UpsertEnvironment(c *fiber.Ctx, envId string) error
	// Delete a team
	// (DELETE /v1/onboarding/environments/{envId}/teams/{teamId})
	DeleteTeam(c *fiber.Ctx, envId string, teamId string) error
	// Create or update a team
	// (PUT /v1/onboarding/environments/{envId}/teams/{teamId})
	UpsertTeam(c *fiber.Ctx, envId string, teamId string) error
	// Delete an app
	// (DELETE /v1/onboarding/environments/{envId}/teams/{teamId}/apps/{appId})
	DeleteApp(c *fiber.Ctx, envId string, teamId string, appId string) error
	// Create or update an app
	// (PUT /v1/onboarding/environments/{envId}/teams/{teamId}/apps/{appId})
	UpsertApp(c *fiber.Ctx, envId string, teamId string, appId string) error
	// Get one or multiple secrets
	// (GET /v1/secrets)
	ListSecrets(c *fiber.Ctx, params ListSecretsParams) error
	// Get a specific secret
	// (GET /v1/secrets/{secretId})
	GetSecret(c *fiber.Ctx, secretId SecretId) error
	// Create or update a secret
	// (PUT /v1/secrets/{secretId})
	PutSecret(c *fiber.Ctx, secretId SecretId) error
}

// ServerInterfaceWrapper converts contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

type MiddlewareFunc fiber.Handler

// DeleteEnvironment operation middleware
func (siw *ServerInterfaceWrapper) DeleteEnvironment(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "envId" -------------
	var envId string

	err = runtime.BindStyledParameterWithOptions("simple", "envId", c.Params("envId"), &envId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter envId: %w", err).Error())
	}

	return siw.Handler.DeleteEnvironment(c, envId)
}

// UpsertEnvironment operation middleware
func (siw *ServerInterfaceWrapper) UpsertEnvironment(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "envId" -------------
	var envId string

	err = runtime.BindStyledParameterWithOptions("simple", "envId", c.Params("envId"), &envId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter envId: %w", err).Error())
	}

	return siw.Handler.UpsertEnvironment(c, envId)
}

// DeleteTeam operation middleware
func (siw *ServerInterfaceWrapper) DeleteTeam(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "envId" -------------
	var envId string

	err = runtime.BindStyledParameterWithOptions("simple", "envId", c.Params("envId"), &envId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter envId: %w", err).Error())
	}

	// ------------- Path parameter "teamId" -------------
	var teamId string

	err = runtime.BindStyledParameterWithOptions("simple", "teamId", c.Params("teamId"), &teamId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter teamId: %w", err).Error())
	}

	return siw.Handler.DeleteTeam(c, envId, teamId)
}

// UpsertTeam operation middleware
func (siw *ServerInterfaceWrapper) UpsertTeam(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "envId" -------------
	var envId string

	err = runtime.BindStyledParameterWithOptions("simple", "envId", c.Params("envId"), &envId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter envId: %w", err).Error())
	}

	// ------------- Path parameter "teamId" -------------
	var teamId string

	err = runtime.BindStyledParameterWithOptions("simple", "teamId", c.Params("teamId"), &teamId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter teamId: %w", err).Error())
	}

	return siw.Handler.UpsertTeam(c, envId, teamId)
}

// DeleteApp operation middleware
func (siw *ServerInterfaceWrapper) DeleteApp(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "envId" -------------
	var envId string

	err = runtime.BindStyledParameterWithOptions("simple", "envId", c.Params("envId"), &envId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter envId: %w", err).Error())
	}

	// ------------- Path parameter "teamId" -------------
	var teamId string

	err = runtime.BindStyledParameterWithOptions("simple", "teamId", c.Params("teamId"), &teamId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter teamId: %w", err).Error())
	}

	// ------------- Path parameter "appId" -------------
	var appId string

	err = runtime.BindStyledParameterWithOptions("simple", "appId", c.Params("appId"), &appId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter appId: %w", err).Error())
	}

	return siw.Handler.DeleteApp(c, envId, teamId, appId)
}

// UpsertApp operation middleware
func (siw *ServerInterfaceWrapper) UpsertApp(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "envId" -------------
	var envId string

	err = runtime.BindStyledParameterWithOptions("simple", "envId", c.Params("envId"), &envId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter envId: %w", err).Error())
	}

	// ------------- Path parameter "teamId" -------------
	var teamId string

	err = runtime.BindStyledParameterWithOptions("simple", "teamId", c.Params("teamId"), &teamId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter teamId: %w", err).Error())
	}

	// ------------- Path parameter "appId" -------------
	var appId string

	err = runtime.BindStyledParameterWithOptions("simple", "appId", c.Params("appId"), &appId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter appId: %w", err).Error())
	}

	return siw.Handler.UpsertApp(c, envId, teamId, appId)
}

// ListSecrets operation middleware
func (siw *ServerInterfaceWrapper) ListSecrets(c *fiber.Ctx) error {

	var err error

	// Parameter object where we will unmarshal all parameters from the context
	var params ListSecretsParams

	var query url.Values
	query, err = url.ParseQuery(string(c.Request().URI().QueryString()))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for query string: %w", err).Error())
	}

	// ------------- Optional query parameter "secretId" -------------

	err = runtime.BindQueryParameter("form", true, false, "secretId", query, &params.SecretId)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter secretId: %w", err).Error())
	}

	return siw.Handler.ListSecrets(c, params)
}

// GetSecret operation middleware
func (siw *ServerInterfaceWrapper) GetSecret(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "secretId" -------------
	var secretId SecretId

	err = runtime.BindStyledParameterWithOptions("simple", "secretId", c.Params("secretId"), &secretId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter secretId: %w", err).Error())
	}

	return siw.Handler.GetSecret(c, secretId)
}

// PutSecret operation middleware
func (siw *ServerInterfaceWrapper) PutSecret(c *fiber.Ctx) error {

	var err error

	// ------------- Path parameter "secretId" -------------
	var secretId SecretId

	err = runtime.BindStyledParameterWithOptions("simple", "secretId", c.Params("secretId"), &secretId, runtime.BindStyledParameterOptions{Explode: false, Required: true})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Errorf("Invalid format for parameter secretId: %w", err).Error())
	}

	return siw.Handler.PutSecret(c, secretId)
}

// FiberServerOptions provides options for the Fiber server.
type FiberServerOptions struct {
	BaseURL     string
	Middlewares []MiddlewareFunc
}

// RegisterHandlers creates http.Handler with routing matching OpenAPI spec.
func RegisterHandlers(router fiber.Router, si ServerInterface) {
	RegisterHandlersWithOptions(router, si, FiberServerOptions{})
}

// RegisterHandlersWithOptions creates http.Handler with additional options
func RegisterHandlersWithOptions(router fiber.Router, si ServerInterface, options FiberServerOptions) {
	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	for _, m := range options.Middlewares {
		router.Use(fiber.Handler(m))
	}

	router.Delete(options.BaseURL+"/v1/onboarding/environments/:envId", wrapper.DeleteEnvironment)

	router.Put(options.BaseURL+"/v1/onboarding/environments/:envId", wrapper.UpsertEnvironment)

	router.Delete(options.BaseURL+"/v1/onboarding/environments/:envId/teams/:teamId", wrapper.DeleteTeam)

	router.Put(options.BaseURL+"/v1/onboarding/environments/:envId/teams/:teamId", wrapper.UpsertTeam)

	router.Delete(options.BaseURL+"/v1/onboarding/environments/:envId/teams/:teamId/apps/:appId", wrapper.DeleteApp)

	router.Put(options.BaseURL+"/v1/onboarding/environments/:envId/teams/:teamId/apps/:appId", wrapper.UpsertApp)

	router.Get(options.BaseURL+"/v1/secrets", wrapper.ListSecrets)

	router.Get(options.BaseURL+"/v1/secrets/:secretId", wrapper.GetSecret)

	router.Put(options.BaseURL+"/v1/secrets/:secretId", wrapper.PutSecret)

}

type ErrorResponseApplicationProblemPlusJSONResponse ApiProblem

type NoContentResponse struct {
}

type OnboardingResponseJSONResponse struct {
	// Items A list of available secret references
	Items []ListSecretItem `json:"items"`
}

type SecretListReponseJSONResponse struct {
	// Items A list of secrets
	Items []Secret `json:"items"`
}

type SecretResponseJSONResponse Secret

type SecretWriteResponseJSONResponse struct {
	// Id A reference to a secret
	Id SecretRef `json:"id"`
}

type DeleteEnvironmentRequestObject struct {
	EnvId string `json:"envId"`
}

type DeleteEnvironmentResponseObject interface {
	VisitDeleteEnvironmentResponse(ctx *fiber.Ctx) error
}

type DeleteEnvironment204Response = NoContentResponse

func (response DeleteEnvironment204Response) VisitDeleteEnvironmentResponse(ctx *fiber.Ctx) error {
	ctx.Status(204)
	return nil
}

type DeleteEnvironment400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response DeleteEnvironment400ApplicationProblemPlusJSONResponse) VisitDeleteEnvironmentResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type DeleteEnvironment500ApplicationProblemPlusJSONResponse ApiProblem

func (response DeleteEnvironment500ApplicationProblemPlusJSONResponse) VisitDeleteEnvironmentResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type UpsertEnvironmentRequestObject struct {
	EnvId string `json:"envId"`
}

type UpsertEnvironmentResponseObject interface {
	VisitUpsertEnvironmentResponse(ctx *fiber.Ctx) error
}

type UpsertEnvironment200JSONResponse struct{ OnboardingResponseJSONResponse }

func (response UpsertEnvironment200JSONResponse) VisitUpsertEnvironmentResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)

	return ctx.JSON(&response)
}

type UpsertEnvironment400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response UpsertEnvironment400ApplicationProblemPlusJSONResponse) VisitUpsertEnvironmentResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type UpsertEnvironment500ApplicationProblemPlusJSONResponse ApiProblem

func (response UpsertEnvironment500ApplicationProblemPlusJSONResponse) VisitUpsertEnvironmentResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type DeleteTeamRequestObject struct {
	EnvId  string `json:"envId"`
	TeamId string `json:"teamId"`
}

type DeleteTeamResponseObject interface {
	VisitDeleteTeamResponse(ctx *fiber.Ctx) error
}

type DeleteTeam204Response = NoContentResponse

func (response DeleteTeam204Response) VisitDeleteTeamResponse(ctx *fiber.Ctx) error {
	ctx.Status(204)
	return nil
}

type DeleteTeam400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response DeleteTeam400ApplicationProblemPlusJSONResponse) VisitDeleteTeamResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type DeleteTeam500ApplicationProblemPlusJSONResponse ApiProblem

func (response DeleteTeam500ApplicationProblemPlusJSONResponse) VisitDeleteTeamResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type UpsertTeamRequestObject struct {
	EnvId  string `json:"envId"`
	TeamId string `json:"teamId"`
}

type UpsertTeamResponseObject interface {
	VisitUpsertTeamResponse(ctx *fiber.Ctx) error
}

type UpsertTeam200JSONResponse struct{ OnboardingResponseJSONResponse }

func (response UpsertTeam200JSONResponse) VisitUpsertTeamResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)

	return ctx.JSON(&response)
}

type UpsertTeam400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response UpsertTeam400ApplicationProblemPlusJSONResponse) VisitUpsertTeamResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type UpsertTeam500ApplicationProblemPlusJSONResponse ApiProblem

func (response UpsertTeam500ApplicationProblemPlusJSONResponse) VisitUpsertTeamResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type DeleteAppRequestObject struct {
	EnvId  string `json:"envId"`
	TeamId string `json:"teamId"`
	AppId  string `json:"appId"`
}

type DeleteAppResponseObject interface {
	VisitDeleteAppResponse(ctx *fiber.Ctx) error
}

type DeleteApp204Response = NoContentResponse

func (response DeleteApp204Response) VisitDeleteAppResponse(ctx *fiber.Ctx) error {
	ctx.Status(204)
	return nil
}

type DeleteApp400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response DeleteApp400ApplicationProblemPlusJSONResponse) VisitDeleteAppResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type DeleteApp500ApplicationProblemPlusJSONResponse ApiProblem

func (response DeleteApp500ApplicationProblemPlusJSONResponse) VisitDeleteAppResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type UpsertAppRequestObject struct {
	EnvId  string `json:"envId"`
	TeamId string `json:"teamId"`
	AppId  string `json:"appId"`
}

type UpsertAppResponseObject interface {
	VisitUpsertAppResponse(ctx *fiber.Ctx) error
}

type UpsertApp200JSONResponse struct{ OnboardingResponseJSONResponse }

func (response UpsertApp200JSONResponse) VisitUpsertAppResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)

	return ctx.JSON(&response)
}

type UpsertApp400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response UpsertApp400ApplicationProblemPlusJSONResponse) VisitUpsertAppResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type UpsertApp500ApplicationProblemPlusJSONResponse ApiProblem

func (response UpsertApp500ApplicationProblemPlusJSONResponse) VisitUpsertAppResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type ListSecretsRequestObject struct {
	Params ListSecretsParams
}

type ListSecretsResponseObject interface {
	VisitListSecretsResponse(ctx *fiber.Ctx) error
}

type ListSecrets200JSONResponse struct{ SecretListReponseJSONResponse }

func (response ListSecrets200JSONResponse) VisitListSecretsResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)

	return ctx.JSON(&response)
}

type ListSecrets400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response ListSecrets400ApplicationProblemPlusJSONResponse) VisitListSecretsResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type ListSecrets500ApplicationProblemPlusJSONResponse ApiProblem

func (response ListSecrets500ApplicationProblemPlusJSONResponse) VisitListSecretsResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type GetSecretRequestObject struct {
	SecretId SecretId `json:"secretId"`
}

type GetSecretResponseObject interface {
	VisitGetSecretResponse(ctx *fiber.Ctx) error
}

type GetSecret200JSONResponse struct{ SecretResponseJSONResponse }

func (response GetSecret200JSONResponse) VisitGetSecretResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)

	return ctx.JSON(&response)
}

type GetSecret400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response GetSecret400ApplicationProblemPlusJSONResponse) VisitGetSecretResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type GetSecret500ApplicationProblemPlusJSONResponse ApiProblem

func (response GetSecret500ApplicationProblemPlusJSONResponse) VisitGetSecretResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

type PutSecretRequestObject struct {
	SecretId SecretId `json:"secretId"`
	Body     *PutSecretJSONRequestBody
}

type PutSecretResponseObject interface {
	VisitPutSecretResponse(ctx *fiber.Ctx) error
}

type PutSecret200JSONResponse struct {
	SecretWriteResponseJSONResponse
}

func (response PutSecret200JSONResponse) VisitPutSecretResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)

	return ctx.JSON(&response)
}

type PutSecret400ApplicationProblemPlusJSONResponse struct {
	ErrorResponseApplicationProblemPlusJSONResponse
}

func (response PutSecret400ApplicationProblemPlusJSONResponse) VisitPutSecretResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(400)

	return ctx.JSON(&response)
}

type PutSecret500ApplicationProblemPlusJSONResponse ApiProblem

func (response PutSecret500ApplicationProblemPlusJSONResponse) VisitPutSecretResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/problem+json")
	ctx.Status(500)

	return ctx.JSON(&response)
}

// StrictServerInterface represents all server handlers.
type StrictServerInterface interface {
	// Delete an environment
	// (DELETE /v1/onboarding/environments/{envId})
	DeleteEnvironment(ctx context.Context, request DeleteEnvironmentRequestObject) (DeleteEnvironmentResponseObject, error)
	// Create or update an environment
	// (PUT /v1/onboarding/environments/{envId})
	UpsertEnvironment(ctx context.Context, request UpsertEnvironmentRequestObject) (UpsertEnvironmentResponseObject, error)
	// Delete a team
	// (DELETE /v1/onboarding/environments/{envId}/teams/{teamId})
	DeleteTeam(ctx context.Context, request DeleteTeamRequestObject) (DeleteTeamResponseObject, error)
	// Create or update a team
	// (PUT /v1/onboarding/environments/{envId}/teams/{teamId})
	UpsertTeam(ctx context.Context, request UpsertTeamRequestObject) (UpsertTeamResponseObject, error)
	// Delete an app
	// (DELETE /v1/onboarding/environments/{envId}/teams/{teamId}/apps/{appId})
	DeleteApp(ctx context.Context, request DeleteAppRequestObject) (DeleteAppResponseObject, error)
	// Create or update an app
	// (PUT /v1/onboarding/environments/{envId}/teams/{teamId}/apps/{appId})
	UpsertApp(ctx context.Context, request UpsertAppRequestObject) (UpsertAppResponseObject, error)
	// Get one or multiple secrets
	// (GET /v1/secrets)
	ListSecrets(ctx context.Context, request ListSecretsRequestObject) (ListSecretsResponseObject, error)
	// Get a specific secret
	// (GET /v1/secrets/{secretId})
	GetSecret(ctx context.Context, request GetSecretRequestObject) (GetSecretResponseObject, error)
	// Create or update a secret
	// (PUT /v1/secrets/{secretId})
	PutSecret(ctx context.Context, request PutSecretRequestObject) (PutSecretResponseObject, error)
}

type StrictHandlerFunc func(ctx *fiber.Ctx, args interface{}) (interface{}, error)

type StrictMiddlewareFunc func(f StrictHandlerFunc, operationID string) StrictHandlerFunc

func NewStrictHandler(ssi StrictServerInterface, middlewares []StrictMiddlewareFunc) ServerInterface {
	return &strictHandler{ssi: ssi, middlewares: middlewares}
}

type strictHandler struct {
	ssi         StrictServerInterface
	middlewares []StrictMiddlewareFunc
}

// DeleteEnvironment operation middleware
func (sh *strictHandler) DeleteEnvironment(ctx *fiber.Ctx, envId string) error {
	var request DeleteEnvironmentRequestObject

	request.EnvId = envId

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.DeleteEnvironment(ctx.UserContext(), request.(DeleteEnvironmentRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "DeleteEnvironment")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(DeleteEnvironmentResponseObject); ok {
		if err := validResponse.VisitDeleteEnvironmentResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// UpsertEnvironment operation middleware
func (sh *strictHandler) UpsertEnvironment(ctx *fiber.Ctx, envId string) error {
	var request UpsertEnvironmentRequestObject

	request.EnvId = envId

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.UpsertEnvironment(ctx.UserContext(), request.(UpsertEnvironmentRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "UpsertEnvironment")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(UpsertEnvironmentResponseObject); ok {
		if err := validResponse.VisitUpsertEnvironmentResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// DeleteTeam operation middleware
func (sh *strictHandler) DeleteTeam(ctx *fiber.Ctx, envId string, teamId string) error {
	var request DeleteTeamRequestObject

	request.EnvId = envId
	request.TeamId = teamId

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.DeleteTeam(ctx.UserContext(), request.(DeleteTeamRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "DeleteTeam")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(DeleteTeamResponseObject); ok {
		if err := validResponse.VisitDeleteTeamResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// UpsertTeam operation middleware
func (sh *strictHandler) UpsertTeam(ctx *fiber.Ctx, envId string, teamId string) error {
	var request UpsertTeamRequestObject

	request.EnvId = envId
	request.TeamId = teamId

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.UpsertTeam(ctx.UserContext(), request.(UpsertTeamRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "UpsertTeam")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(UpsertTeamResponseObject); ok {
		if err := validResponse.VisitUpsertTeamResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// DeleteApp operation middleware
func (sh *strictHandler) DeleteApp(ctx *fiber.Ctx, envId string, teamId string, appId string) error {
	var request DeleteAppRequestObject

	request.EnvId = envId
	request.TeamId = teamId
	request.AppId = appId

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.DeleteApp(ctx.UserContext(), request.(DeleteAppRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "DeleteApp")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(DeleteAppResponseObject); ok {
		if err := validResponse.VisitDeleteAppResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// UpsertApp operation middleware
func (sh *strictHandler) UpsertApp(ctx *fiber.Ctx, envId string, teamId string, appId string) error {
	var request UpsertAppRequestObject

	request.EnvId = envId
	request.TeamId = teamId
	request.AppId = appId

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.UpsertApp(ctx.UserContext(), request.(UpsertAppRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "UpsertApp")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(UpsertAppResponseObject); ok {
		if err := validResponse.VisitUpsertAppResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// ListSecrets operation middleware
func (sh *strictHandler) ListSecrets(ctx *fiber.Ctx, params ListSecretsParams) error {
	var request ListSecretsRequestObject

	request.Params = params

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.ListSecrets(ctx.UserContext(), request.(ListSecretsRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "ListSecrets")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(ListSecretsResponseObject); ok {
		if err := validResponse.VisitListSecretsResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// GetSecret operation middleware
func (sh *strictHandler) GetSecret(ctx *fiber.Ctx, secretId SecretId) error {
	var request GetSecretRequestObject

	request.SecretId = secretId

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.GetSecret(ctx.UserContext(), request.(GetSecretRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "GetSecret")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(GetSecretResponseObject); ok {
		if err := validResponse.VisitGetSecretResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// PutSecret operation middleware
func (sh *strictHandler) PutSecret(ctx *fiber.Ctx, secretId SecretId) error {
	var request PutSecretRequestObject

	request.SecretId = secretId

	var body PutSecretJSONRequestBody
	if err := ctx.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	request.Body = &body

	handler := func(ctx *fiber.Ctx, request interface{}) (interface{}, error) {
		return sh.ssi.PutSecret(ctx.UserContext(), request.(PutSecretRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "PutSecret")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(PutSecretResponseObject); ok {
		if err := validResponse.VisitPutSecretResponse(ctx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	res := make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	resolvePath := PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		pathToFile := url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
