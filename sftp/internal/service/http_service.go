// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

// HTTPService implements Service using the generated SFTP Tardis OpenAPI client.
type HTTPService struct {
	client ClientWithResponsesInterface
}

// Config configures an HTTPService.
type Config struct {
	Endpoint   *url.URL
	HTTPClient *http.Client
	Generation int64
}

// NewHTTPService creates an SFTP Tardis HTTP service.
func NewHTTPService(cfg Config) (*HTTPService, error) {
	client, err := newClientWithResponses(cfg)
	if err != nil {
		return nil, err
	}
	return &HTTPService{client: client}, nil
}

func newClientWithResponses(cfg Config) (ClientWithResponsesInterface, error) {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	generatedClient, err := NewClientWithResponses(cfg.Endpoint.String(),
		WithHTTPClient(httpClient),
		WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Accept", "application/json")
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("creating SFTP Tardis client: %w", err)
	}

	return generatedClient, nil
}

func (s *HTTPService) CreateOrUpdateSFTPUser(ctx context.Context, user RoverSftpUserModel) error {
	res, err := s.client.CreateOrUpdateSftpUserWithResponse(ctx, user)
	if err != nil {
		return ctrlerrors.RetryableErrorf("SFTP Tardis API request failed: %s", err.Error())
	}

	switch res.StatusCode() {
	case http.StatusOK, http.StatusCreated:
		return nil
	default:
		return handleAPIError("create or update SFTP user", res.StatusCode(), res.Body, firstAPIError(res.JSON400, res.JSON500))
	}
}

func (s *HTTPService) UpdatePublicKeysForSFTPUser(ctx context.Context, sftpUserName, clientID string, keys ClientPublicKeyMap) error {
	res, err := s.client.UpdatePublicKeysForSftpUserWithResponse(ctx, sftpUserName, &UpdatePublicKeysForSftpUserParams{
		ClientId: clientID,
	}, keys)
	if err != nil {
		return ctrlerrors.RetryableErrorf("SFTP Tardis API request failed: %s", err.Error())
	}

	switch res.StatusCode() {
	case http.StatusOK:
		return nil
	default:
		return handleAPIError("update SFTP user public keys", res.StatusCode(), res.Body, firstAPIError(res.JSON400, res.JSON500))
	}
}

func (s *HTTPService) DeleteSFTPUser(ctx context.Context, sftpUserName string) error {
	res, err := s.client.DeleteSftpUserWithResponse(ctx, sftpUserName)
	if err != nil {
		return ctrlerrors.RetryableErrorf("SFTP Tardis API request failed: %s", err.Error())
	}

	switch res.StatusCode() {
	case http.StatusOK:
		return nil
	default:
		return handleAPIError("delete SFTP user", res.StatusCode(), res.Body, firstAPIError(res.JSON400, res.JSON500))
	}
}
