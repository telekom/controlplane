// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2/clientcredentials"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// HTTPServiceFactory manages HTTP services configured from SFTPServiceConfig resources.
type HTTPServiceFactory struct {
	mu       sync.RWMutex
	services map[string]cachedService
}

type cachedService struct {
	generation int64
	service    Service
}

func NewHTTPServiceFactory() *HTTPServiceFactory {
	return &HTTPServiceFactory{
		services: make(map[string]cachedService),
	}
}

func (f *HTTPServiceFactory) ServiceFor(ctx context.Context, sftpServiceConfig client.ObjectKey) (Service, error) {
	cacheKey := sftpServiceConfigCacheKey(sftpServiceConfig)

	f.mu.RLock()
	cached, ok := f.services[cacheKey]
	f.mu.RUnlock()
	if !ok {
		return nil, ctrlerrors.RetryableErrorf("SFTP client for SFTPServiceConfig %q is not initialized", cacheKey)
	}

	return cached.service, nil
}

func (f *HTTPServiceFactory) ExistClient(sftpServiceConfig client.ObjectKey) bool {
	cacheKey := sftpServiceConfigCacheKey(sftpServiceConfig)

	f.mu.RLock()
	_, ok := f.services[cacheKey]
	f.mu.RUnlock()
	return ok
}

func (f *HTTPServiceFactory) CreateOrUpdate(ctx context.Context, sftpServiceConfig *sftpv1.SFTPServiceConfig) error {
	cacheKey := sftpServiceConfigCacheKey(client.ObjectKeyFromObject(sftpServiceConfig))

	f.mu.RLock()
	cached, ok := f.services[cacheKey]
	f.mu.RUnlock()
	if ok && cached.generation == sftpServiceConfig.Generation {
		return nil
	}

	cfg, err := f.ClientConfigFor(ctx, sftpServiceConfig)
	if err != nil {
		return err
	}

	service, err := NewHTTPService(cfg)
	if err != nil {
		return err
	}

	f.mu.Lock()
	if f.services == nil {
		f.services = make(map[string]cachedService)
	}
	f.services[cacheKey] = cachedService{
		generation: cfg.Generation,
		service:    service,
	}
	f.mu.Unlock()
	return nil
}

func (f *HTTPServiceFactory) ClientConfigFor(ctx context.Context, sftpServiceConfig *sftpv1.SFTPServiceConfig) (Config, error) {
	return clientConfigFor(ctx, sftpServiceConfig)
}

func (f *HTTPServiceFactory) Delete(sftpServiceConfig *sftpv1.SFTPServiceConfig) {
	cacheKey := sftpServiceConfigCacheKey(client.ObjectKeyFromObject(sftpServiceConfig))

	f.mu.Lock()
	delete(f.services, cacheKey)
	f.mu.Unlock()
}

func clientConfigFor(ctx context.Context, sftpServiceConfig *sftpv1.SFTPServiceConfig) (Config, error) {
	if sftpServiceConfig == nil {
		return Config{}, fmt.Errorf("sftpServiceConfig is nil")
	}

	oauth2Config, err := clientCredentials(ctx, sftpServiceConfig.Spec.API)
	if err != nil {
		return Config{}, err
	}

	log := logf.FromContext(ctx)
	log.V(1).Info("Fetching SFTP Tardis access token")
	_, err = oauth2Config.Token(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("fetching SFTP Tardis access token: %w", err)
	}

	endpointURL, err := parseBaseURL(sftpServiceConfig.Spec.API.Endpoint)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Endpoint:   endpointURL,
		HTTPClient: oauth2Config.Client(context.Background()),
		Generation: sftpServiceConfig.Generation,
	}

	cfg.HTTPClient.Timeout = 30 * time.Second

	return cfg, nil
}

func sftpServiceConfigCacheKey(sftpServiceConfig client.ObjectKey) string {
	return sftpServiceConfig.String()
}

func clientCredentials(ctx context.Context, api sftpv1.APIEndpoint) (*clientcredentials.Config, error) {
	clientID := strings.TrimSpace(api.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("SFTP Tardis client ID must not be empty")
	}

	tokenURL := strings.TrimSpace(api.Issuer)
	if tokenURL == "" {
		return nil, fmt.Errorf("SFTP Tardis token endpoint must not be empty")
	}

	clientSecret := strings.TrimSpace(api.ClientSecret)
	if clientSecret == "" {
		return nil, fmt.Errorf("SFTP Tardis client secret must not be empty")
	}

	if secretsapi.IsRef(clientSecret) {
		var err error
		clientSecret, err = secretsapi.API().Get(ctx, clientSecret)
		if err != nil {
			return nil, fmt.Errorf("getting SFTP Tardis client secret from secret-manager: %w", err)
		}
	}

	clientSecret = strings.TrimSpace(clientSecret)
	if strings.TrimSpace(clientSecret) == "" {
		return nil, fmt.Errorf("SFTP Tardis client secret must not be empty")
	}

	return &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
	}, nil
}
