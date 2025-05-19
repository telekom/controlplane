// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/secret-manager/internal/api"
	"github.com/telekom/controlplane/secret-manager/pkg/controller"
)

var _ api.StrictServerInterface = &Handler{}

type Handler struct {
	ctrl controller.Controller
}

func NewHandler(ctrl controller.Controller) *Handler {
	return &Handler{
		ctrl: ctrl,
	}
}

func (h *Handler) GetSecret(ctx context.Context, req api.GetSecretRequestObject) (res api.GetSecretResponseObject, err error) {
	secret, err := h.ctrl.GetSecret(ctx, req.SecretId)
	if err != nil {
		return res, err
	}
	okRes := api.GetSecret200JSONResponse{
		SecretResponseJSONResponse: api.SecretResponseJSONResponse{
			Id:    secret.Id,
			Value: secret.Value,
		},
	}

	return okRes, nil
}

func (h *Handler) ListSecrets(ctx context.Context, req api.ListSecretsRequestObject) (api.ListSecretsResponseObject, error) {
	logr.FromContextOrDiscard(ctx).Info("ListSecrets", "request", req)
	return api.ListSecrets200JSONResponse{
		SecretListReponseJSONResponse: api.SecretListReponseJSONResponse{
			Items: []api.Secret{
				{
					Id:    "123",
					Value: "123",
				},
			},
		},
	}, nil
}

func (h *Handler) PutSecret(ctx context.Context, req api.PutSecretRequestObject) (api.PutSecretResponseObject, error) {
	secret, err := h.ctrl.SetSecret(ctx, req.SecretId, req.Body.Value)
	if err != nil {
		return nil, err
	}
	okRes := api.PutSecret200JSONResponse{
		SecretWriteResponseJSONResponse: api.SecretWriteResponseJSONResponse{
			Id: secret.Id,
		},
	}
	return okRes, nil
}

func (h *Handler) UpsertEnvironment(ctx context.Context, request api.UpsertEnvironmentRequestObject) (api.UpsertEnvironmentResponseObject, error) {
	res, err := h.ctrl.OnboardEnvironment(ctx, request.EnvId)
	if err != nil {
		return nil, err
	}

	okRes := api.UpsertEnvironment200JSONResponse{
		OnboardingResponseJSONResponse: api.OnboardingResponseJSONResponse{
			Items: mapOnboardingResponseItems(res.SecretRefs),
		},
	}
	return okRes, nil
}

func (h *Handler) UpsertTeam(ctx context.Context, request api.UpsertTeamRequestObject) (api.UpsertTeamResponseObject, error) {
	res, err := h.ctrl.OnboardTeam(ctx, request.EnvId, request.TeamId)
	if err != nil {
		return nil, err
	}

	okRes := api.UpsertTeam200JSONResponse{
		OnboardingResponseJSONResponse: api.OnboardingResponseJSONResponse{
			Items: mapOnboardingResponseItems(res.SecretRefs),
		},
	}
	return okRes, nil
}

func (h *Handler) UpsertApp(ctx context.Context, request api.UpsertAppRequestObject) (api.UpsertAppResponseObject, error) {
	res, err := h.ctrl.OnboardApplication(ctx, request.EnvId, request.TeamId, request.AppId)
	if err != nil {
		return nil, err
	}

	okRes := api.UpsertApp200JSONResponse{
		OnboardingResponseJSONResponse: api.OnboardingResponseJSONResponse{
			Items: mapOnboardingResponseItems(res.SecretRefs),
		},
	}
	return okRes, nil
}

func (h *Handler) DeleteEnvironment(ctx context.Context, request api.DeleteEnvironmentRequestObject) (api.DeleteEnvironmentResponseObject, error) {
	err := h.ctrl.DeleteEnvironment(ctx, request.EnvId)
	if err != nil {
		return nil, err
	}
	return api.DeleteEnvironment204Response{}, nil
}

func (h *Handler) DeleteTeam(ctx context.Context, request api.DeleteTeamRequestObject) (api.DeleteTeamResponseObject, error) {
	err := h.ctrl.DeleteTeam(ctx, request.EnvId, request.TeamId)
	if err != nil {
		return nil, err
	}
	return api.DeleteTeam204Response{}, nil
}

func (h *Handler) DeleteApp(ctx context.Context, request api.DeleteAppRequestObject) (api.DeleteAppResponseObject, error) {
	err := h.ctrl.DeleteApplication(ctx, request.EnvId, request.TeamId, request.AppId)
	if err != nil {
		return nil, err

	}
	return api.DeleteApp204Response{}, nil
}

func mapOnboardingResponseItems(items map[string]string) []api.ListSecretItem {
	list := make([]api.ListSecretItem, 0, len(items))
	for name, ref := range items {
		list = append(list, api.ListSecretItem{
			Name: name,
			Id:   ref,
		})
	}
	return list
}
