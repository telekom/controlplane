// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/secret-manager/api/accesstoken"
	"github.com/telekom/controlplane/secret-manager/api/gen"
	"github.com/telekom/controlplane/secret-manager/api/util"
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

type SecretsApi interface {
	Get(ctx context.Context, secretID string) (value string, err error)
	Set(ctx context.Context, secretID string, secretValue string) (newID string, err error)
	Rotate(ctx context.Context, secretID string) (newID string, err error)
}

type OnboardingApi interface {
	UpsertEnvironment(ctx context.Context, envID string) (availableSecrets []gen.ListSecretItem, err error)
	UpsertTeam(ctx context.Context, envID, teamID string) (availableSecrets []gen.ListSecretItem, err error)
	UpsertApplication(ctx context.Context, envID, teamID, appID string) (availableSecrets []gen.ListSecretItem, err error)

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

type Options struct {
	URL           string
	Token         accesstoken.AccessToken
	SkipTLSVerify bool
}

func (o *Options) accessTokenReqEditor(ctx context.Context, req *http.Request) error {
	if o.Token == nil {
		return nil
	}
	token, err := o.Token.Read()
	if err != nil {
		return errors.Wrap(err, "failed to read access token")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return nil
}

func defaultOptions() *Options {
	if util.IsRunningInCluster() {
		return &Options{
			URL:   inCluster,
			Token: accesstoken.NewAccessToken(accesstoken.TokenFilePath),
		}
	} else {
		return &Options{
			URL:   localhost,
			Token: nil,
		}
	}
}

type Option func(*Options)

func WithURL(url string) Option {
	return func(o *Options) {
		o.URL = url
	}
}

func WithAccessToken(token accesstoken.AccessToken) Option {
	return func(o *Options) {
		o.Token = token
	}
}

func WithSkipTLSVerify() Option {
	return func(o *Options) {
		o.SkipTLSVerify = true
	}
}

func NewOnboarding(opts ...Option) OnboardingApi {
	return New(opts...)
}

func NewSecrets(opts ...Option) SecretsApi {
	return New(opts...)
}

func New(opts ...Option) SecretManager {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	if !strings.HasPrefix(options.URL, "https://") {
		fmt.Println("⚠️\tWarning: Using HTTP instead of HTTPS. This is not secure.")
	}
	skipTlsVerify := os.Getenv("SKIP_TLS_VERIFY") == "true" || options.SkipTLSVerify
	httpClient, err := gen.NewClientWithResponses(options.URL, gen.WithHTTPClient(util.NewHttpClientOrDie(skipTlsVerify, CaFilePath)), gen.WithRequestEditorFn(options.accessTokenReqEditor))
	if err != nil {
		panic(fmt.Sprintf("Failed to create client: %v", err))
	}
	return &secretManagerAPI{
		client: httpClient,
	}
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

func (s *secretManagerAPI) UpsertEnvironment(ctx context.Context, envID string) (availableSecrets []gen.ListSecretItem, err error) {
	res, err := s.client.UpsertEnvironmentWithResponse(ctx, envID)
	if err != nil {
		return nil, err
	}
	switch res.StatusCode() {
	case 200:
		return res.JSON200.Items, nil
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

func (s *secretManagerAPI) UpsertTeam(ctx context.Context, envID, teamID string) (availableSecrets []gen.ListSecretItem, err error) {
	res, err := s.client.UpsertTeamWithResponse(ctx, envID, teamID)
	if err != nil {
		return nil, err
	}
	switch res.StatusCode() {
	case 200:
		return res.JSON200.Items, nil
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

func (s *secretManagerAPI) UpsertApplication(ctx context.Context, envID, teamID, appID string) (availableSecrets []gen.ListSecretItem, err error) {
	res, err := s.client.UpsertAppWithResponse(ctx, envID, teamID, appID)
	if err != nil {
		return nil, err
	}
	switch res.StatusCode() {
	case 200:
		return res.JSON200.Items, nil
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
func FindSecretId(items []gen.ListSecretItem, name string) (string, bool) {
	for _, item := range items {
		if item.Name == name {
			return ToRef(item.Id), true
		}
	}
	return "", false
}

// FromRef will strip the tags from the given string if it is a placeholder.
// Otherwise, it will return the string as is.
func FromRef(ref string) (string, bool) {
	if !strings.HasPrefix(ref, StartTag) || !strings.HasSuffix(ref, EndTag) {
		return ref, false
	}
	ref = strings.TrimPrefix(ref, StartTag)
	ref = strings.TrimSuffix(ref, EndTag)
	return ref, true
}

// ToRef will add the tags to the given string.
// If it is already a placeholder, it will return the string as is.
func ToRef(id string) string {
	if strings.HasPrefix(id, StartTag) && strings.HasSuffix(id, EndTag) {
		return id
	}
	return StartTag + id + EndTag
}
