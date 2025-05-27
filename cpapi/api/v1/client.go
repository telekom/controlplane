// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const tokenUrlPath = "/protocol/openid-connect/token"

type ClientConfig interface {
	GetUrl() string
	GetClientSecret() string
	GetClientId() string
	GetIssuerUrl() string
}

func GetApiClient(ctx context.Context, config ClientConfig) (ClientWithResponsesInterface, error) {
	baseClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
		Timeout: 10 * time.Second,
	}

	tokenCfg := clientcredentials.Config{
		ClientID:     config.GetClientId(),
		ClientSecret: config.GetClientSecret(),
		TokenURL:     config.GetIssuerUrl() + tokenUrlPath,
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, baseClient)

	url, err := url.Parse(config.GetUrl())
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse URL")
	}

	httpClient := tokenCfg.Client(ctx)

	apiClient, err := NewClientWithResponses(url.String(), WithHTTPClient(httpClient))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create api client")
	}
	return apiClient, nil
}

// Deprecated: no longer maintained
func EvalResponse(statusCode int, body []byte) error {
	if statusCode < 299 {
		return nil
	}

	err := &Error{}
	if err := json.Unmarshal(body, err); err != nil {
		return errors.Wrap(err, "failed to unmarshal error")
	}

	return errors.Errorf("error in response: %s", *err.Detail) // TODO: maybe refactor when error handling is done
}
