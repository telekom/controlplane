// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/secret-manager/api/gen"
)

const (
	// Using port-forward to the secret manager API in the cluster
	localhost = "https://localhost:9090/api"
	inCluster = "https://secret-manager.controlplane-system.svc.cluster.local/api"
	StartTag  = "$<"
	EndTag    = ">"

	CaFilePath    = "/var/run/secrets/trust-bundle/trust-bundle.pem"
	TokenFilePath = "/var/run/secrets/secretmgr/token"

	// KeywordRotate is a special keyword to indicate that the secret should be rotated.
	KeywordRotate = "rotate"
)

var (
	// ErrNotFound is returned when a secret is not found in the secret manager.
	ErrNotFound = client.BlockedErrorf("resource not found")
)

type OnboardingOptions struct {
	SecretValues map[string]any
	Strategy     *gen.WriteStrategy
}

type OnboardingOption func(*OnboardingOptions)

// WithSecretValue allows you to set a secret value for the onboarding process.
// The secret name must be known to the secret manager.
// Example: If the secret manager has a secret named "externalSecrets",
// Then you can set the value directly or create sub-secrets like so
// WithSecretValue("externalSecrets.foo.my-secret", "my-secret-value").
// IMPORTANT: The {{rotate}} keyword is not supported here.
func WithSecretValue(name string, value any) OnboardingOption {
	return func(o *OnboardingOptions) {
		if o.SecretValues == nil {
			o.SecretValues = make(map[string]any)
		}
		o.SecretValues[name] = value
	}
}

// WithStrategy sets the write strategy for the onboarding process.
// "merge" preserves existing secrets not in the request.
// "replace" (default) drops existing secrets not in the request.
func WithStrategy(strategy gen.WriteStrategy) OnboardingOption {
	return func(o *OnboardingOptions) {
		o.Strategy = &strategy
	}
}

// WithMergeStrategy sets the onboarding strategy to "merge", which preserves existing secrets not in the request.
func WithMergeStrategy() OnboardingOption {
	return WithStrategy(gen.Merge)
}

// WithReplaceStrategy sets the onboarding strategy to "replace", which drops existing secrets not in the request.
func WithReplaceStrategy() OnboardingOption {
	return WithStrategy(gen.Replace)
}

type SecretsApi interface {
	Get(ctx context.Context, secretID string) (value string, err error)
	Set(ctx context.Context, secretID string, secretValue string) (newID string, err error)
	Rotate(ctx context.Context, secretID string) (newID string, err error)
}

type OnboardingApi interface {
	UpsertEnvironment(ctx context.Context, envID string, opts ...OnboardingOption) (availableSecrets map[string]string, err error)
	UpsertTeam(ctx context.Context, envID, teamID string, opts ...OnboardingOption) (availableSecrets map[string]string, err error)
	UpsertApplication(ctx context.Context, envID, teamID, appID string, opts ...OnboardingOption) (availableSecrets map[string]string, err error)

	DeleteEnvironment(ctx context.Context, envID string) (err error)
	DeleteTeam(ctx context.Context, envID, teamID string) (err error)
	DeleteApplication(ctx context.Context, envID, teamID, appID string) (err error)
}

type SecretManager interface {
	SecretsApi
	OnboardingApi
}

var _ SecretManager = (*secretManagerAPI)(nil)

type secretManagerAPI struct {
	client gen.ClientWithResponsesInterface
}

// NewSecretManagerFromClient creates a SecretManager from an existing generated client.
// This is primarily useful for testing with mock clients.
func NewSecretManagerFromClient(client gen.ClientWithResponsesInterface) SecretManager {
	return &secretManagerAPI{client: client}
}

func (s *secretManagerAPI) Get(ctx context.Context, secretID string) (value string, err error) {
	// Remove the tags from the secret ID if it is a placeholder.
	// If it is not a placeholder, we just assume that it is a valid secret ID.
	secretID, _ = FromRef(secretID)
	res, err := s.client.GetSecretWithResponse(ctx, secretID)
	if err != nil {
		return "", fmt.Errorf("secret-manager request failed for %q: %w", secretID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK:
		return res.JSON200.Value, nil
	case http.StatusNotFound:
		return "", ErrNotFound
	case http.StatusUnauthorized:
		return "", client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return "", handleError(res.StatusCode(), string(res.Body))
	}
}
func (s *secretManagerAPI) Set(ctx context.Context, secretID string, secretValue string) (newID string, err error) {
	// Remove the tags from the secret ID if it is a placeholder.
	// If it is not a placeholder, we just assume that it is a valid secret ID.
	secretID, _ = FromRef(secretID)
	res, err := s.client.PutSecretWithResponse(ctx, secretID, gen.PutSecretJSONRequestBody{Value: secretValue})
	if err != nil {
		return "", fmt.Errorf("secret-manager request failed for %q: %w", secretID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK:
		return ToRef(res.JSON200.Id), nil
	case http.StatusNoContent:
		return secretID, nil
	case http.StatusNotFound:
		return "", ErrNotFound
	case http.StatusUnauthorized:
		return "", client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return "", handleError(res.StatusCode(), string(res.Body))
	}
}

func (s *secretManagerAPI) Rotate(ctx context.Context, secretID string) (newID string, err error) {
	return s.Set(ctx, secretID, KeywordRotate)
}

func (s *secretManagerAPI) UpsertEnvironment(ctx context.Context, envID string, opts ...OnboardingOption) (availableSecrets map[string]string, err error) {
	options := &OnboardingOptions{}
	for _, opt := range opts {
		opt(options)
	}

	reqBody := gen.UpsertEnvironmentJSONRequestBody{}
	reqBody.Secrets, err = toNamedSecrets(options.SecretValues)
	if err != nil {
		return nil, err
	}
	reqBody.Strategy = options.Strategy

	res, err := s.client.UpsertEnvironmentWithResponse(ctx, envID, reqBody)
	if err != nil {
		return nil, fmt.Errorf("secret-manager request failed for environment %q: %w", envID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK:
		return toMap(res.JSON200.Items), nil
	case http.StatusNoContent:
		return nil, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusUnauthorized:
		return nil, client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return nil, handleError(res.StatusCode(), string(res.Body))
	}
}

func (s *secretManagerAPI) UpsertTeam(ctx context.Context, envID, teamID string, opts ...OnboardingOption) (availableSecrets map[string]string, err error) {
	options := &OnboardingOptions{}
	for _, opt := range opts {
		opt(options)
	}

	reqBody := gen.UpsertTeamJSONRequestBody{}
	reqBody.Secrets, err = toNamedSecrets(options.SecretValues)
	if err != nil {
		return nil, err
	}
	reqBody.Strategy = options.Strategy

	res, err := s.client.UpsertTeamWithResponse(ctx, envID, teamID, reqBody)
	if err != nil {
		return nil, fmt.Errorf("secret-manager request failed for team %q/%q: %w", envID, teamID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK:
		return toMap(res.JSON200.Items), nil
	case http.StatusNoContent:
		return nil, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusUnauthorized:
		return nil, client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return nil, handleError(res.StatusCode(), string(res.Body))
	}
}

func (s *secretManagerAPI) UpsertApplication(ctx context.Context, envID, teamID, appID string, opts ...OnboardingOption) (availableSecrets map[string]string, err error) {
	options := &OnboardingOptions{}
	for _, opt := range opts {
		opt(options)
	}

	reqBody := gen.UpsertAppJSONRequestBody{}
	reqBody.Secrets, err = toNamedSecrets(options.SecretValues)
	if err != nil {
		return nil, err
	}
	reqBody.Strategy = options.Strategy

	res, err := s.client.UpsertAppWithResponse(ctx, envID, teamID, appID, reqBody)
	if err != nil {
		return nil, fmt.Errorf("secret-manager request failed for app %q/%q/%q: %w", envID, teamID, appID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK:
		return toMap(res.JSON200.Items), nil
	case http.StatusNoContent:
		return nil, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusUnauthorized:
		return nil, client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return nil, handleError(res.StatusCode(), string(res.Body))
	}
}

func (s *secretManagerAPI) DeleteEnvironment(ctx context.Context, envID string) (err error) {
	res, err := s.client.DeleteEnvironmentWithResponse(ctx, envID)
	if err != nil {
		return fmt.Errorf("secret-manager request failed for environment %q: %w", envID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized:
		return client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return handleError(res.StatusCode(), string(res.Body))
	}
}

func (s *secretManagerAPI) DeleteTeam(ctx context.Context, envID, teamID string) (err error) {
	res, err := s.client.DeleteTeamWithResponse(ctx, envID, teamID)
	if err != nil {
		return fmt.Errorf("secret-manager request failed for team %q/%q: %w", envID, teamID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized:
		return client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return handleError(res.StatusCode(), string(res.Body))
	}
}

func (s *secretManagerAPI) DeleteApplication(ctx context.Context, envID, teamID, appID string) (err error) {
	res, err := s.client.DeleteAppWithResponse(ctx, envID, teamID, appID)
	if err != nil {
		return fmt.Errorf("secret-manager request failed for app %q/%q/%q: %w", envID, teamID, appID, client.RetryableErrorf("network error: %s", err))
	}
	switch res.StatusCode() {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized:
		return client.BlockedErrorf("unauthorized (%d): %s", res.StatusCode(), string(res.Body))
	default:
		return handleError(res.StatusCode(), string(res.Body))
	}
}

// handleError classifies HTTP status codes from the secret-manager API following
// HTTP semantics: 4xx errors (client errors) are blocked because retrying with the
// same parameters will not succeed; 5xx errors are retryable (transient server failures).
// 408 (Request Timeout) and 429 (Too Many Requests) are special 4xx codes that are
// retryable and are handled by client.HandleError.
func handleError(httpStatus int, msg string) error {
	// 408 and 429 are retryable 4xx codes — delegate to client.HandleError which handles them.
	if httpStatus == http.StatusRequestTimeout || httpStatus == http.StatusTooManyRequests {
		return client.HandleError(httpStatus, msg)
	}
	// All other 4xx are client errors — blocked, retrying won't help.
	if httpStatus >= 400 && httpStatus < 500 {
		return client.BlockedErrorf("client error (%d): %s", httpStatus, msg)
	}
	// 5xx and anything else — delegate to client.HandleError.
	return client.HandleError(httpStatus, msg)
}

// FindSecretId will find the secret ID for the given name in the list of secrets.
// It will automatically convert the secret ID to a secret Reference.
func FindSecretId(availableSecrets map[string]string, name string) (string, bool) {
	if id, ok := availableSecrets[name]; ok {
		return ToRef(id), true
	}
	return "", false
}

// FromRef will strip the tags from the given string if it is a placeholder.
// Otherwise, it will return the string as is.
func FromRef(ref string) (string, bool) {
	if !IsRef(ref) {
		return ref, false
	}
	ref = strings.TrimPrefix(ref, StartTag)
	ref = strings.TrimSuffix(ref, EndTag)
	return ref, true
}

func IsRef(ref string) bool {
	return strings.HasPrefix(ref, StartTag) && strings.HasSuffix(ref, EndTag)
}

// ToRef will add the tags to the given string.
// If it is already a placeholder, it will return the string as is.
func ToRef(id string) string {
	if strings.HasPrefix(id, StartTag) && strings.HasSuffix(id, EndTag) {
		return id
	}
	return StartTag + id + EndTag
}

func toMap(items []gen.ListSecretItem) map[string]string {
	secretMap := make(map[string]string)
	for _, item := range items {
		secretMap[item.Name] = ToRef(item.Id)
	}
	return secretMap
}

func toNamedSecrets(secretValues map[string]any) ([]gen.NamedSecret, error) {
	secrets := make([]gen.NamedSecret, 0, len(secretValues))
	for name, value := range secretValues {
		switch v := value.(type) {
		case string:
			secrets = append(secrets, gen.NamedSecret{
				Name:  name,
				Value: v,
			})
		case map[string]any:
			jsonValue, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal secret value for %s: %w", name, err)
			}
			secrets = append(secrets, gen.NamedSecret{
				Name:  name,
				Value: string(jsonValue),
			})
		default:
			return nil, fmt.Errorf("unsupported secret value type for %s: %T", name, value)
		}
	}

	return secrets, nil
}
