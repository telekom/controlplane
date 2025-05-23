// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
)

var _ KeycloakClient = &keycloakClient{}

type keycloakClient struct {
	KeycloakClient
}

type KeycloakClient interface {
	GetRealmWithResponse(ctx context.Context, realm string,
		reqEditors ...RequestEditorFn) (*GetRealmResponse, error)
	PutRealmWithResponse(ctx context.Context, realm string, body PutRealmJSONRequestBody,
		reqEditors ...RequestEditorFn) (*PutRealmResponse, error)
	PostWithResponse(ctx context.Context, body PostJSONRequestBody,
		reqEditors ...RequestEditorFn) (*PostResponse, error)
	DeleteRealmWithResponse(ctx context.Context, realm string,
		reqEditors ...RequestEditorFn) (*DeleteRealmResponse, error)

	GetRealmClientsWithResponse(ctx context.Context, realm string, params *GetRealmClientsParams,
		reqEditors ...RequestEditorFn) (*GetRealmClientsResponse, error)
	PutRealmClientsIdWithResponse(ctx context.Context, realm string, id string, body PutRealmClientsIdJSONRequestBody,
		reqEditors ...RequestEditorFn) (*PutRealmClientsIdResponse, error)
	PostRealmClientsWithResponse(ctx context.Context, realm string, body PostRealmClientsJSONRequestBody,
		reqEditors ...RequestEditorFn) (*PostRealmClientsResponse, error)
	DeleteRealmClientsIdWithResponse(ctx context.Context, realm, id string,
		reqEditors ...RequestEditorFn) (*DeleteRealmClientsIdResponse, error)
}
