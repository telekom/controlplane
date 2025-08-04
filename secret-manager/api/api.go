// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/secret-manager/api/gen"
)

const (
	localhost = "http://localhost:9090/api"
	inCluster = "https://secret-manager.secret-manager-system.svc.cluster.local/api"
	StartTag  = "$<"
	EndTag    = ">"

	CaFilePath = "/var/run/secrets/trust-bundle/trust-bundle.pem"

	// KeywordRotate is a special keyword to indicate that the secret should be rotated.
	KeywordRotate = "rotate"
)

var (
	ErrNotFound = errors.New("resource not found")
)

type OnboardingOptions struct {
	SecretValues map[string]any
}

type OnboardingOption func(*OnboardingOptions)

func WithSecretValue(name string, value any) OnboardingOption {
	return func(o *OnboardingOptions) {
		if o.SecretValues == nil {
			o.SecretValues = make(map[string]any)
		}
		o.SecretValues[name] = value
	}
}

type SecretsApi interface {
	Get(ctx context.Context, secretID string) (value string, err error)
	Set(ctx context.Context, secretID string, secretValue string) (newID string, err error)
	Rotate(ctx context.Context, secretID string) (newID string, err error)
}

type OnboardingApi interface {
	UpsertEnvironment(ctx context.Context, envID string) (availableSecrets map[string]string, err error)
	UpsertTeam(ctx context.Context, envID, teamID string) (availableSecrets map[string]string, err error)
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

func (s *secretManagerAPI) Get(ctx context.Context, secretID string) (value string, err error) {
	// Remove the tags from the secret ID if it is a placeholder.
	// If it is not a placeholder, we just assume that it is a valid secret ID.
	secretID, _ = FromRef(secretID)
	res, err := s.client.GetSecretWithResponse(ctx, secretID)
	if err != nil {
		return "", err
	}
	switch res.StatusCode() {
	case 200:
		return res.JSON200.Value, nil
	case 404:
		return "", ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return "", err
		}
		return "", fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}
func (s *secretManagerAPI) Set(ctx context.Context, secretID string, secretValue string) (newID string, err error) {
	// Remove the tags from the secret ID if it is a placeholder.
	// If it is not a placeholder, we just assume that it is a valid secret ID.
	secretID, _ = FromRef(secretID)
	res, err := s.client.PutSecretWithResponse(ctx, secretID, gen.PutSecretJSONRequestBody{Value: secretValue})
	if err != nil {
		return "", err
	}
	switch res.StatusCode() {
	case 200:
		return ToRef(res.JSON200.Id), nil
	case 204:
		return secretID, nil
	case 404:
		return "", ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return "", err
		}
		return "", fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (s *secretManagerAPI) Rotate(ctx context.Context, secretID string) (newID string, err error) {
	return s.Set(ctx, secretID, KeywordRotate)
}

func (s *secretManagerAPI) UpsertEnvironment(ctx context.Context, envID string) (availableSecrets map[string]string, err error) {
	res, err := s.client.UpsertEnvironmentWithResponse(ctx, envID)
	if err != nil {
		return nil, err
	}
	switch res.StatusCode() {
	case 200:
		return toMap(res.JSON200.Items), nil
	case 204:
		return nil, nil
	case 404:
		return nil, ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (s *secretManagerAPI) UpsertTeam(ctx context.Context, envID, teamID string) (availableSecrets map[string]string, err error) {
	res, err := s.client.UpsertTeamWithResponse(ctx, envID, teamID)
	if err != nil {
		return nil, err
	}
	switch res.StatusCode() {
	case 200:
		return toMap(res.JSON200.Items), nil
	case 204:
		return nil, nil
	case 404:
		return nil, ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (s *secretManagerAPI) UpsertApplication(ctx context.Context, envID, teamID, appID string, opts ...OnboardingOption) (availableSecrets map[string]string, err error) {
	options := &OnboardingOptions{}
	for _, opt := range opts {
		opt(options)
	}

	reqBody := gen.UpsertAppJSONRequestBody{
		Secrets: &[]gen.NamedSecret{},
	}
	for name, value := range options.SecretValues {
		switch v := value.(type) {
		case string:
			*reqBody.Secrets = append(*reqBody.Secrets, gen.NamedSecret{
				Name:  name,
				Value: v,
			})
		case map[string]any:
			jsonValue, err := json.Marshal(v)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal secret value for %s", name)
			}
			*reqBody.Secrets = append(*reqBody.Secrets, gen.NamedSecret{
				Name:  name,
				Value: string(jsonValue),
			})
		default:
			return nil, fmt.Errorf("unsupported secret value type for %s: %T", name, value)
		}
	}

	res, err := s.client.UpsertAppWithResponse(ctx, envID, teamID, appID, reqBody)
	if err != nil {
		return nil, err
	}
	switch res.StatusCode() {
	case 200:
		return toMap(res.JSON200.Items), nil
	case 204:
		return nil, nil
	case 404:
		return nil, ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (s *secretManagerAPI) DeleteEnvironment(ctx context.Context, envID string) (err error) {
	res, err := s.client.DeleteEnvironmentWithResponse(ctx, envID)
	if err != nil {
		return err
	}
	switch res.StatusCode() {
	case 200:
		return nil
	case 204:
		return nil
	case 404:
		return nil
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return err
		}
		return fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (s *secretManagerAPI) DeleteTeam(ctx context.Context, envID, teamID string) (err error) {
	res, err := s.client.DeleteTeamWithResponse(ctx, envID, teamID)
	if err != nil {
		return err
	}
	switch res.StatusCode() {
	case 200:
		return nil
	case 204:
		return nil
	case 404:
		return nil
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return err
		}
		return fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (s *secretManagerAPI) DeleteApplication(ctx context.Context, envID, teamID, appID string) (err error) {
	res, err := s.client.DeleteAppWithResponse(ctx, envID, teamID, appID)
	if err != nil {
		return err
	}
	switch res.StatusCode() {
	case 200:
		return nil
	case 204:
		return nil
	case 404:
		return nil
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(res.Body, &err); err != nil {
			return err
		}
		return fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

// FindSecretId will find the secret ID for the given name in the list of secrets.
// It will automatically convert the secret ID to a reference.
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
