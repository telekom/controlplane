// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

// Package publication publishes complete xDS bundles to the management server.
package publication

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
)

const (
	defaultTimeout    = 5 * time.Second
	defaultAttempts   = 3
	defaultRetryDelay = 100 * time.Millisecond
)

type publishClient interface {
	PublishBundle(context.Context, *xdsapi.PublishBundleRequest, ...grpc.CallOption) (*xdsapi.PublishBundleResponse, error)
}

// Publisher retries transient publication failures with one deadline per attempt.
type Publisher struct {
	client     publishClient
	timeout    time.Duration
	attempts   int
	retryDelay time.Duration
}

// New creates a publisher using conservative POC retry defaults.
func New(client xdsapi.PublicationServiceClient) *Publisher {
	return &Publisher{
		client:     client,
		timeout:    defaultTimeout,
		attempts:   defaultAttempts,
		retryDelay: defaultRetryDelay,
	}
}

// Publish sends the same immutable envelope on every retry.
func (p *Publisher) Publish(ctx context.Context, bundle *xdsapi.Bundle) (*xdsapi.PublishBundleResponse, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle is required")
	}

	var lastErr error
	for attempt := 1; attempt <= p.attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, p.timeout)
		response, err := p.client.PublishBundle(attemptCtx, &xdsapi.PublishBundleRequest{Bundle: bundle})
		cancel()
		if err == nil {
			return response, nil
		}

		lastErr = err
		if !IsRetryable(err) || attempt == p.attempts {
			break
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("publishing bundle: %w", ctx.Err())
		case <-time.After(p.retryDelay):
		}
	}

	return nil, fmt.Errorf("publishing bundle: %w", publicationError(lastErr))
}

func publicationError(err error) error {
	grpcStatus, ok := status.FromError(err)
	if !ok {
		return err
	}
	details := grpcStatus.Details()
	messages := make([]string, 0, len(details))
	for _, detail := range details {
		if validationError, ok := detail.(*xdsapi.ValidationError); ok {
			messages = append(messages, validationError.Field+": "+validationError.Message)
		}
	}
	if len(messages) == 0 {
		return err
	}
	return status.Error(grpcStatus.Code(), grpcStatus.Message()+" ("+strings.Join(messages, "; ")+")")
}

// IsRetryable identifies failures for which replaying an idempotent request is safe.
func IsRetryable(err error) bool {
	switch status.Code(err) {
	case codes.Aborted, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Unavailable:
		return true
	default:
		return false
	}
}
