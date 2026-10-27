// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"github.com/telekom/controlplane/common-server/pkg/client/tlsclient"
	"github.com/telekom/controlplane/common/pkg/cabundle"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"
)

type GatewayAdminConfig interface {
	AdminUrl() string
	AdminClientId() string
	AdminClientSecret() string
	AdminIssuer() string
}

type gatewayAdminConfig struct {
	url          string
	clientId     string
	clientSecret string
	issuer       string
}

func (g *gatewayAdminConfig) AdminUrl() string {
	return g.url
}

func (g *gatewayAdminConfig) AdminClientId() string {
	return g.clientId
}

func (g *gatewayAdminConfig) AdminClientSecret() string {
	return g.clientSecret
}

func (g *gatewayAdminConfig) AdminIssuer() string {
	return g.issuer
}

func NewGatewayConfig(rawUrl, clientId, clientSecret, issuer string) GatewayAdminConfig {
	return &gatewayAdminConfig{
		url:          rawUrl,
		clientId:     clientId,
		clientSecret: clientSecret,
		issuer:       issuer,
	}
}

const (
	tokenUrlPath = "/protocol/openid-connect/token"
	profileName  = "gateway"
	ownerKind    = "gateway.cp.ei.telekom.de/Gateway"
)

// profile holds the HTTP settings of all Gateway admin API clients.
func profile() tlsclient.Profile {
	return tlsclient.Profile{
		Name:                profileName,
		Transport:           tlsclient.TransportRaw,
		Timeout:             tlsclient.DefaultTimeout,
		MaxIdleConnsPerHost: 100,
		MetricsClientName:   "gateway",
		MetricsReplacePatterns: []string{
			`([^\/]+--[^\/]+--[^\/]+)`, `([^\/]+--[^\/]+)`, metrics.ReplacePatternUID,
		},
	}
}

func specFor(gwCfg GatewayAdminConfig) (tlsclient.Spec, error) {
	tokenUrl, err := url.JoinPath(gwCfg.AdminIssuer(), tokenUrlPath)
	if err != nil {
		return tlsclient.Spec{}, fmt.Errorf("failed to construct token URL: %w", err)
	}

	return tlsclient.Spec{
		Profile: profileName,
		APIURL:  gwCfg.AdminUrl(),
		Auth: tlsclient.Auth{
			Mode:         tlsclient.AuthClientCredentials,
			TokenURL:     tokenUrl,
			ClientID:     gwCfg.AdminClientId(),
			ClientSecret: gwCfg.AdminClientSecret(),
		},
	}, nil
}

// ClientFactory provides Kong clients for Gateways. A single instance is
// shared by all controllers of the operator and must be added to the manager.
type ClientFactory struct {
	clients *tlsclient.Factory
	reader  client.Reader
}

// NewClientFactory creates the factory. ctx is the operator lifetime and
// reader is used to read CA bundle ConfigMaps (the manager cached client).
func NewClientFactory(ctx context.Context, reader client.Reader, opts ...tlsclient.FactoryOption) (*ClientFactory, error) {
	clients, err := tlsclient.NewFactory(ctx, []tlsclient.Profile{profile()}, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating kong client factory: %w", err)
	}
	return &ClientFactory{clients: clients, reader: reader}, nil
}

// Start runs the cleanup of idle clients. It implements manager.Runnable.
func (f *ClientFactory) Start(ctx context.Context) error {
	return f.clients.Start(ctx)
}

// NeedLeaderElection lets the cleanup run on every replica.
func (f *ClientFactory) NeedLeaderElection() bool {
	return false
}

// ClientFor returns a Kong client for the Gateway and a release function
// that must be called once the client is no longer used.
// Invalid CA configuration results in a blocked error.
func (f *ClientFactory) ClientFor(ctx context.Context, gw *gatewayv1.Gateway) (kongclient.KongClient, func(), error) {
	lease, err := f.acquire(ctx, gw)
	if err != nil {
		return nil, nil, err
	}
	apiClient, err := kong.NewClientWithResponses(lease.Client().APIURL(), kong.WithHTTPClient(lease.Client().Doer()))
	if err != nil {
		lease.Release()
		return nil, nil, fmt.Errorf("creating kong client: %w", err)
	}
	return kongclient.NewKongClient(apiClient), lease.Release, nil
}

func (f *ClientFactory) acquire(ctx context.Context, gw *gatewayv1.Gateway) (*tlsclient.Lease, error) {
	if gw == nil {
		return nil, errors.New("gateway is required")
	}
	spec, err := specFor(gw)
	if err != nil {
		return nil, err
	}
	spec.Owner = tlsclient.Owner{
		Kind:      ownerKind,
		Namespace: gw.Namespace,
		Name:      gw.Name,
		UID:       string(gw.UID),
	}
	if gw.Spec.TLS != nil {
		if f.reader == nil {
			return nil, fmt.Errorf("gateway %s/%s references a CA bundle but no resolver is configured", gw.Namespace, gw.Name)
		}
		spec.CA = cabundle.NewResolver(f.reader, gw.Namespace, gw.Spec.TLS.ConfigMapRef)
	}

	lease, err := f.clients.Acquire(ctx, spec)
	if err != nil {
		return nil, classifyAcquireError(gw, err)
	}
	return lease, nil
}

// classifyAcquireError blocks on invalid CA configuration. Other errors are
// returned wrapped and retried.
func classifyAcquireError(gw *gatewayv1.Gateway, err error) error {
	if errors.Is(err, cabundle.ErrInvalidBundle) ||
		errors.Is(err, cabundle.ErrSourceNotFound) ||
		errors.Is(err, cabundle.ErrSourceForbidden) ||
		errors.Is(err, tlsclient.ErrInvalidCA) {
		return ctrlerrors.BlockedErrorf("invalid CA configuration of gateway %s/%s: %s", gw.Namespace, gw.Name, err)
	}
	return fmt.Errorf("acquiring kong client for gateway %s/%s: %w", gw.Namespace, gw.Name, err)
}

// GetClientFor returns a leased Kong client of factory for the Gateway.
// The returned release function must be called when done. It is a test seam.
var GetClientFor = func(ctx context.Context, factory *ClientFactory, gw *gatewayv1.Gateway) (kongclient.KongClient, func(), error) {
	if factory == nil {
		return nil, nil, errors.New("kong client factory is not configured")
	}
	return factory.ClientFor(ctx, gw)
}

// NewStandaloneClient creates an uncached Kong admin API client for
// long-running tools. ctx bounds the token source lifetime; the returned
// close function releases its resources. insecureSkipVerify disables TLS
// verification for the token and API endpoints.
func NewStandaloneClient(ctx context.Context, gwCfg GatewayAdminConfig, insecureSkipVerify bool) (kong.ClientWithResponsesInterface, func(), error) {
	spec, err := specFor(gwCfg)
	if err != nil {
		return nil, nil, err
	}
	c, err := tlsclient.New(ctx, profile(), spec, tlsclient.WithInsecureSkipVerify(insecureSkipVerify))
	if err != nil {
		return nil, nil, fmt.Errorf("creating kong http client: %w", err)
	}
	apiClient, err := kong.NewClientWithResponses(c.APIURL(), kong.WithHTTPClient(c.Doer()))
	if err != nil {
		c.Close()
		return nil, nil, fmt.Errorf("creating kong client: %w", err)
	}
	return apiClient, c.Close, nil
}
