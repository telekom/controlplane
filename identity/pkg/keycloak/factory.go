// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
	k8stypes "k8s.io/apimachinery/pkg/types"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

const requestTimeout = 10 * time.Second

// ServiceFactory returns KeycloakService instances for the connection recorded in a Realm
// status. Services may only be used for the current reconciliation.
type ServiceFactory interface {
	ServiceFor(ctx context.Context, realm *identityv1.Realm) (KeycloakService, error)
}

// ServiceFactoryFunc adapts a plain function to the ServiceFactory interface. Useful for tests.
type ServiceFactoryFunc func(ctx context.Context, realm *identityv1.Realm) (KeycloakService, error)

func (f ServiceFactoryFunc) ServiceFor(ctx context.Context, realm *identityv1.Realm) (KeycloakService, error) {
	return f(ctx, realm)
}

// Connection holds transient runtime values; it is never persisted.
type Connection struct {
	AdminUrl      string
	AdminTokenUrl string
	ClientId      string
	UserName      string
	Password      string
	CA            *commonclient.ProviderCABundle
}

// ServiceBuilder builds a service for conn. ctx is the reconcile context for initial
// authentication; refreshCtx lives as long as the client cache. closeIdle releases idle connections.
type ServiceBuilder func(ctx, refreshCtx context.Context, conn *Connection) (svc KeycloakService, closeIdle func(), err error)

// FactoryOption configures the production factory.
type FactoryOption func(*Factory)

// WithServiceBuilder replaces the HTTP client stack builder.
func WithServiceBuilder(b ServiceBuilder) FactoryOption {
	return func(f *Factory) { f.build = b }
}

// Factory is the production ServiceFactory. It uses the connection fields of the Realm passed
// by the caller, which it never rereads, and acquires the service from the shared client cache.
type Factory struct {
	clients *ClientCache
	build   ServiceBuilder
}

var _ ServiceFactory = &Factory{}

// NewServiceFactory creates the production factory. clients is the manager's shared Keycloak
// client cache and is required.
func NewServiceFactory(clients *ClientCache, opts ...FactoryOption) *Factory {
	if clients == nil {
		panic("keycloak.NewServiceFactory: client cache is required")
	}
	f := &Factory{clients: clients, build: NewKeycloakServiceFor}
	for _, o := range opts {
		o(f)
	}
	return f
}

// ServiceFor implements ServiceFactory. The client cache replaces the service when the Realm
// UID, connection, credentials or CA bundle change; callers holding the previous service may
// finish their requests with it.
func (f *Factory) ServiceFor(ctx context.Context, realm *identityv1.Realm) (KeycloakService, error) {
	if realm == nil {
		return nil, errors.New("realm is nil")
	}
	key := k8stypes.NamespacedName{Namespace: realm.Namespace, Name: realm.Name}
	status := &realm.Status
	if identityv1.ClassifyRealmSnapshot(status) != identityv1.RealmSnapshotComplete {
		return nil, ctrlerrors.BlockedErrorf("realm %s has no complete connection status", key.String())
	}
	// Checked before any CA resolution, secret resolution, cache use or network request.
	if err := RequireSnapshotHTTPS(status); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %s connection status: %s", key.String(), err)
	}
	password, err := secrets.Get(ctx, status.AdminPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve password from secret manager: %w", err)
	}
	conn := Connection{
		AdminUrl:      status.AdminUrl,
		AdminTokenUrl: status.AdminTokenUrl,
		ClientId:      status.AdminClientId,
		UserName:      status.AdminUserName,
		Password:      password,
	}
	ref := status.TLS.Ref()
	request := providerca.ClientRequest{
		Owner: key,
		UID:   realm.UID,
		Parts: []string{conn.AdminUrl, conn.AdminTokenUrl, conn.ClientId, conn.UserName, conn.Password},
		Ref:   ref,
	}
	svc, err := f.clients.Acquire(ctx, request, func(callCtx, lifetimeCtx context.Context, bundle *commonclient.ProviderCABundle) (providerca.Built[KeycloakService], error) {
		c := conn
		c.CA = bundle
		built, closeIdle, buildErr := f.build(callCtx, lifetimeCtx, &c)
		if buildErr != nil {
			return providerca.Built[KeycloakService]{}, buildErr
		}
		logr.FromContextOrDiscard(ctx).V(1).Info("Built keycloak service for realm", "realm", key.String())
		return providerca.Built[KeycloakService]{Value: built, CloseIdle: closeIdle}, nil
	})
	if providerca.AsResolveError(err) != nil {
		return nil, mapCAError(realm.Namespace, ref, err)
	}
	return svc, err
}

// NewKeycloakServiceFor builds the HTTP client stack: one trust-aware transport shared by
// the token and API clients, both rejecting redirects. Initial authentication uses ctx;
// token refresh and re-authentication use refreshCtx.
func NewKeycloakServiceFor(ctx, refreshCtx context.Context, conn *Connection) (KeycloakService, func(), error) {
	if conn == nil {
		return nil, nil, errors.New("connection is nil")
	}
	tokenUrl, err := url.Parse(conn.AdminTokenUrl)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse token URL: %w", err)
	}
	endpointUrl, err := url.Parse(conn.AdminUrl)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse keycloak admin URL: %w", err)
	}

	baseClient := commonclient.NewBaseHttpClient(commonclient.WithClientTimeout(requestTimeout))
	baseTransport, ok := baseClient.Transport.(*http.Transport)
	if !ok {
		return nil, nil, errors.New("base HTTP transport is not *http.Transport")
	}
	transport, err := commonclient.NewProviderTransport(baseTransport, conn.CA)
	if err != nil {
		return nil, nil, fmt.Errorf("building provider transport: %w", err)
	}
	closeIdle := transport.CloseIdleConnections
	tokenClient := &http.Client{Transport: transport, Timeout: baseClient.Timeout, CheckRedirect: commonclient.DenyRedirects}

	oauth2Cfg := &oauth2.Config{
		ClientID: conn.ClientId,
		Endpoint: oauth2.Endpoint{TokenURL: tokenUrl.String()},
	}
	token, err := oauth2Cfg.PasswordCredentialsToken(context.WithValue(ctx, oauth2.HTTPClient, tokenClient), conn.UserName, conn.Password)
	if err != nil {
		closeIdle()
		return nil, nil, fmt.Errorf("failed to retrieve token: %w", classifyOAuth2Error(err))
	}

	refresh := context.WithValue(refreshCtx, oauth2.HTTPClient, tokenClient)
	ts := oauth2.ReuseTokenSource(token, newReAuthTokenSource(refresh, oauth2Cfg, conn.UserName, conn.Password, token))
	httpClient := &http.Client{
		Transport:     &oauth2.Transport{Source: ts, Base: transport},
		Timeout:       baseClient.Timeout,
		CheckRedirect: commonclient.DenyRedirects,
	}

	metricsClient := metrics.WithMetrics(httpClient,
		metrics.WithClientName("identity"),
		metrics.WithReplacePatterns(
			`([^\/]+--[^\/]+--[^\/]+)`,
			`([^\/]+--[^\/]+)`,
			metrics.ReplacePatternUID,
		),
	)

	apiClient, err := api.NewClientWithResponses(endpointUrl.String(), api.WithHTTPClient(metricsClient))
	if err != nil {
		closeIdle()
		return nil, nil, fmt.Errorf("failed to create keycloak admin client: %w", err)
	}
	return NewKeycloakService(apiClient), closeIdle, nil
}
