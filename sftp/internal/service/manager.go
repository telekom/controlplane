// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2/clientcredentials"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// HTTPServiceFactory manages HTTP services configured from ZoneServiceConfig resources.
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

func (f *HTTPServiceFactory) ServiceFor(ctx context.Context, zsc client.ObjectKey) (Service, error) {
	cacheKey := zoneServiceConfigCacheKey(zsc)

	f.mu.RLock()
	cached, ok := f.services[cacheKey]
	f.mu.RUnlock()
	if !ok {
		return nil, ctrlerrors.RetryableErrorf("SFTP client for ZoneServiceConfig %q is not initialized", cacheKey)
	}

	return cached.service, nil
}

func (f *HTTPServiceFactory) IsServiceCached(zsc client.ObjectKey) bool {
	cacheKey := zoneServiceConfigCacheKey(zsc)

	f.mu.RLock()
	_, ok := f.services[cacheKey]
	f.mu.RUnlock()
	return ok
}

func (f *HTTPServiceFactory) CreateOrUpdate(ctx context.Context, zsc *sftpv1.ZoneServiceConfig) error {
	cacheKey := zoneServiceConfigCacheKey(client.ObjectKeyFromObject(zsc))

	f.mu.RLock()
	cached, ok := f.services[cacheKey]
	f.mu.RUnlock()
	if ok && cached.generation == zsc.Generation {
		return nil
	}

	cfg, err := f.ClientConfigFor(ctx, zsc)
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

func (f *HTTPServiceFactory) ClientConfigFor(ctx context.Context, zsc *sftpv1.ZoneServiceConfig) (Config, error) {
	return clientConfigFor(ctx, zsc)
}

func (f *HTTPServiceFactory) Delete(zsc *sftpv1.ZoneServiceConfig) {
	cacheKey := zoneServiceConfigCacheKey(client.ObjectKeyFromObject(zsc))

	f.mu.Lock()
	delete(f.services, cacheKey)
	f.mu.Unlock()
}

func clientConfigFor(ctx context.Context, zsc *sftpv1.ZoneServiceConfig) (Config, error) {
	if zsc == nil {
		return Config{}, fmt.Errorf("zoneServiceConfig is nil")
	}

	oauth2Config, err := clientCredentials(ctx, zsc.Spec.API)
	if err != nil {
		return Config{}, err
	}

	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Fetching SFTP Tardis access token")
	_, err = oauth2Config.Token(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("fetching SFTP Tardis access token: %w", err)
	}

	endpointURL, err := parseBaseURL(zsc.Spec.API.Endpoint)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Endpoint:   endpointURL,
		HTTPClient: oauth2Config.Client(context.Background()),
		Generation: zsc.Generation,
	}

	cfg.HTTPClient.Timeout = 30 * time.Second

	return cfg, nil
}

func zoneServiceConfigCacheKey(zsc client.ObjectKey) string {
	return zsc.String()
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
