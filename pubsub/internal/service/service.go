// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/pkg/errors"

	"github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
)

const (
	pathGVR = "/subscriber.horizon.telekom.de/v1/subscriptions"
)

type ConfigService interface {
	PutSubscription(ctx context.Context, subscriptionID string, resource SubscriptionResource) error
	DeleteSubscription(ctx context.Context, subscriptionID string, resource SubscriptionResource) error
}

type ConfigServiceConfig struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
}

var _ ConfigService = &configService{}

type configService struct {
	BasePath   string
	httpClient metrics.HttpRequestDoer
}

func NewConfigService(config ConfigServiceConfig) ConfigService {
	httpClient := NewAuthorizedHttpClient(context.Background(), config.TokenURL, config.ClientID, config.ClientSecret)

	metricsClient := metrics.WithMetrics(httpClient,
		metrics.WithClientName("pubsub"),
		metrics.WithReplacePatterns(`[0-9a-f]{40}`),
	)

	return &configService{BasePath: config.BaseURL, httpClient: metricsClient}
}

func (q *configService) buildURL(subscriptionID string) (*url.URL, error) {
	if subscriptionID == "" {
		return nil, errors.New("subscriptionID is required to build URL")
	}
	rawUrl := q.BasePath + pathGVR + "/" + subscriptionID
	return url.Parse(rawUrl)
}

func (q *configService) DeleteSubscription(ctx context.Context, subscriptionID string, resource SubscriptionResource) error { //nolint:gocritic // hugeParam: kept as value to match interface
	reqURL, err := q.buildURL(subscriptionID)
	if err != nil {
		return errors.Wrap(err, "failed to build URL for DeleteSubscription")
	}

	body, err := json.Marshal(resource)
	if err != nil {
		return errors.Wrap(err, "failed to serialize resource for DeleteSubscription")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "failed to create HTTP request for DeleteSubscription")
	}
	resp, err := q.httpClient.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "HTTP request failed for DeleteSubscription")
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response body

	return checkResponse(resp, "DeleteSubscription", http.StatusOK, http.StatusNoContent)
}

func (q *configService) PutSubscription(ctx context.Context, subscriptionID string, resource SubscriptionResource) error { //nolint:gocritic // hugeParam: kept as value to match interface
	reqURL, err := q.buildURL(subscriptionID)
	if err != nil {
		return errors.Wrap(err, "failed to build URL for PutSubscription")
	}

	body, err := json.Marshal(resource)
	if err != nil {
		return errors.Wrap(err, "failed to serialize resource for PutSubscription")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "failed to create HTTP request for PutSubscription")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := q.httpClient.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "HTTP request failed for PutSubscription")
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response body

	return checkResponse(resp, "PutSubscription", http.StatusOK, http.StatusCreated)
}

func checkResponse(resp *http.Response, operation string, okStatusCodes ...int) error {
	msg := fmt.Sprintf("operation %q failed", operation)
	respContent, err := io.ReadAll(resp.Body)
	if err == nil {
		msg = fmt.Sprintf("%s: %q", msg, string(respContent))
	}

	return client.HandleError(resp.StatusCode, msg, okStatusCodes...)
}
