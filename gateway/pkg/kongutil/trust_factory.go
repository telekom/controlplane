// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	ktypes "k8s.io/apimachinery/pkg/types"

	ptls "github.com/telekom/controlplane/common-server/pkg/client"
	metrics "github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/urlutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"
)

const (
	// RequestTimeout is the per-request timeout of Kong API and token clients.
	RequestTimeout = 10 * time.Second
	tokenURLPath   = "/protocol/openid-connect/token"
)

// TrustError is a configuration fault of a referenced CA bundle.
type TrustError struct {
	Reason string
	err    *ctrlerrors.CtrlError
}

func (e *TrustError) Error() string { return e.err.Error() }

// Unwrap exposes the controller error so ctrlerrors.HandleError classifies it.
func (e *TrustError) Unwrap() error { return e.err }

// NewTrustError wraps a controller error as trust fault with a condition reason.
func NewTrustError(reason string, err *ctrlerrors.CtrlError) *TrustError {
	return &TrustError{Reason: reason, err: err}
}

func trustBlocked(reason, format string, a ...any) error {
	return &TrustError{Reason: reason, err: ctrlerrors.BlockedErrorf(format, a...)}
}

func trustRetryable(reason, format string, a ...any) error {
	return &TrustError{Reason: reason, err: ctrlerrors.RetryableErrorf(format, a...)}
}

// AsTrustError returns the TrustError in err's chain, or nil.
func AsTrustError(err error) *TrustError {
	var te *TrustError
	if errors.As(err, &te) {
		return te
	}
	return nil
}

// SecretResolver resolves a secret reference into its value.
type SecretResolver func(ctx context.Context, ref string) (string, error)

// CAResolver resolves Gateway CA references; it is the manager's shared CA cache.
type CAResolver = providerca.CAResolver[*ptls.ProviderCABundle]

// ClientCache is the shared cache of Kong clients.
type ClientCache = providerca.ClientCache[kongclient.KongClient, *ptls.ProviderCABundle]

// NewClientCache returns the Kong client cache resolving trust through ca.
func NewClientCache(ca CAResolver) *ClientCache {
	return providerca.NewClientCache[kongclient.KongClient, *ptls.ProviderCABundle](ca)
}

// Factory acquires trust-aware Kong clients from the shared client cache for the supplied
// Gateway. Acquired clients may only be used for the current reconciliation.
type Factory struct {
	ca            CAResolver
	clients       *ClientCache
	resolveSecret SecretResolver
}

// NewFactory creates a Factory. ca and clients are required and should share the CA cache.
// A nil resolveSecret uses the client secret value as is.
func NewFactory(ca CAResolver, clients *ClientCache, resolveSecret SecretResolver) *Factory {
	if ca == nil || clients == nil {
		panic("kongutil.NewFactory: CA resolver and client cache are required")
	}
	return &Factory{ca: ca, clients: clients, resolveSecret: resolveSecret}
}

// Validate checks the Gateway endpoints and resolves its trust without credentials or clients.
func (f *Factory) Validate(ctx context.Context, gw *gatewayv1.Gateway) error {
	if gw == nil {
		return errors.New("validating gateway trust: gateway is nil")
	}
	if err := validateEndpoints(gw); err != nil {
		return err
	}
	ref := gw.Spec.Admin.TLS.Ref()
	if _, err := f.ca.Resolve(ctx, gw.Namespace, ref); err != nil {
		return mapCAError(gw.Namespace, ref, err)
	}
	return nil
}

// ClientFor implements ClientFactory.
func (f *Factory) ClientFor(ctx context.Context, gw *gatewayv1.Gateway) (kongclient.KongClient, error) {
	if gw == nil {
		return nil, errors.New("acquiring kong client: gateway is nil")
	}
	if err := validateEndpoints(gw); err != nil {
		return nil, err
	}
	admin := gw.Spec.Admin
	secret := admin.ClientSecret
	if f.resolveSecret != nil {
		var err error
		if secret, err = f.resolveSecret(ctx, admin.ClientSecret); err != nil {
			return nil, fmt.Errorf("resolving gateway client secret: %w", err)
		}
	}
	ref := admin.TLS.Ref()
	request := providerca.ClientRequest{
		Owner: ktypes.NamespacedName{Namespace: gw.Namespace, Name: gw.Name},
		UID:   gw.UID,
		Parts: []string{admin.Url, admin.IssuerUrl, admin.ClientId, secret},
		Ref:   ref,
	}
	c, err := f.clients.Acquire(ctx, request, func(_, lifetimeCtx context.Context, bundle *ptls.ProviderCABundle) (providerca.Built[kongclient.KongClient], error) {
		return build(lifetimeCtx, admin.Url, admin.IssuerUrl, admin.ClientId, secret, bundle)
	})
	if providerca.AsResolveError(err) != nil {
		return nil, mapCAError(gw.Namespace, ref, err)
	}
	return c, err
}

// validateEndpoints requires both the API and the issuer URL to be https when custom trust is
// configured, since both use the same trust.
func validateEndpoints(gw *gatewayv1.Gateway) error {
	admin := gw.Spec.Admin
	if admin.TLS.Ref() == nil {
		return nil
	}
	if err := validateCustomTrustURL(admin.Url); err != nil {
		return trustBlocked("InvalidEndpoint", "API URL with custom trust: %s", err)
	}
	if err := validateCustomTrustURL(admin.IssuerUrl); err != nil {
		return trustBlocked("InvalidEndpoint", "issuer URL with custom trust: %s", err)
	}
	return nil
}

// validateCustomTrustURL requires an absolute https URL with a hostname, no userinfo and no fragment.
func validateCustomTrustURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parsing %q: %w", raw, err)
	}
	switch {
	case !u.IsAbs() || !urlutil.IsHTTPS(u):
		return errors.New("must be an absolute https URL")
	case u.Hostname() == "":
		return errors.New("must have a hostname")
	case u.User != nil:
		return errors.New("must not contain userinfo")
	case u.Fragment != "" || strings.Contains(raw, "#"):
		return errors.New("must not contain a fragment")
	}
	return nil
}

// mapCAError maps a shared CA resolution failure to a TrustError.
func mapCAError(namespace string, ref *ctypes.CABundleConfigMapRef, err error) error {
	re := providerca.AsResolveError(err)
	if re == nil {
		return trustRetryable("ConfigMapReadFailed", "resolving CA ConfigMap %s/%s: %s", namespace, ref.Name, err)
	}
	switch re.Kind {
	case providerca.FailureInvalidReference:
		return trustBlocked("InvalidReference", "CA reference: %s", re.Err)
	case providerca.FailureNotFound:
		return trustBlocked("ConfigMapNotFound", "CA ConfigMap %s/%s not found or lacks label %s=%s",
			namespace, ref.Name, ctypes.ProviderCALabelKey, ctypes.ProviderCALabelValue)
	case providerca.FailureForbidden:
		return trustBlocked("ConfigMapForbidden", "CA ConfigMap %s/%s is forbidden", namespace, ref.Name)
	case providerca.FailureLabelMissing:
		return trustBlocked("LabelMissing", "CA ConfigMap %s/%s %s", namespace, ref.Name, re.Err)
	case providerca.FailureInvalidMetadata:
		return trustBlocked("InvalidProvenance", "CA ConfigMap %s/%s %s", namespace, ref.Name, re.Err)
	case providerca.FailureKeyMissing:
		return trustBlocked("KeyMissing", "CA ConfigMap %s/%s %s", namespace, ref.Name, re.Err)
	case providerca.FailureInvalidBundle:
		return trustBlocked("InvalidBundle", "CA ConfigMap %s/%s key %q: %s", namespace, ref.Name, ref.Key, re.Err)
	default:
		return trustRetryable("ConfigMapReadFailed", "reading CA ConfigMap %s/%s: %s", namespace, ref.Name, re.Err)
	}
}

func baseTransport() *http.Transport {
	t := &http.Transport{}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		t = dt.Clone()
	}
	t.MaxIdleConnsPerHost = 100
	return t
}

// build creates a Kong client over one trust-aware transport. No network I/O happens here.
func build(lifetimeCtx context.Context, adminURL, issuer, clientID, secret string, ca *ptls.ProviderCABundle) (providerca.Built[kongclient.KongClient], error) {
	var none providerca.Built[kongclient.KongClient]
	u, err := url.Parse(adminURL)
	if err != nil {
		return none, ctrlerrors.BlockedErrorf("parsing gateway admin URL: %s", err)
	}
	transport, err := ptls.NewProviderTransport(baseTransport(), ca)
	if err != nil {
		return none, ctrlerrors.BlockedErrorf("building transport: %s", err)
	}
	closeIdle := transport.CloseIdleConnections
	apiHTTP := newAPIClient(lifetimeCtx, transport, issuer, clientID, secret)
	metricsClient := metrics.WithMetrics(apiHTTP,
		metrics.WithClientName("gateway"),
		metrics.WithReplacePatterns(`([^\/]+--[^\/]+--[^\/]+)`, `([^\/]+--[^\/]+)`, metrics.ReplacePatternUID),
	)
	apiClient, err := kong.NewClientWithResponses(u.String(), kong.WithHTTPClient(metricsClient))
	if err != nil {
		closeIdle()
		return none, fmt.Errorf("creating kong client: %w", err)
	}
	return providerca.Built[kongclient.KongClient]{Value: kongclient.NewKongClient(apiClient), CloseIdle: closeIdle}, nil
}

// newAPIClient returns an API client adding the access token. Its token source fetches tokens
// over a distinct client sharing transport, refreshing with lifetimeCtx, never a reconcile
// context.
func newAPIClient(lifetimeCtx context.Context, transport http.RoundTripper, issuer, clientID, secret string) *http.Client {
	token := &http.Client{Transport: transport, Timeout: RequestTimeout, CheckRedirect: ptls.DenyRedirects}
	tokenCfg := clientcredentials.Config{ClientID: clientID, ClientSecret: secret, TokenURL: issuer + tokenURLPath}
	tokenCtx := context.WithValue(lifetimeCtx, oauth2.HTTPClient, token)
	return &http.Client{
		Transport:     &oauth2.Transport{Source: oauth2.ReuseTokenSource(nil, tokenCfg.TokenSource(tokenCtx)), Base: transport},
		Timeout:       RequestTimeout,
		CheckRedirect: ptls.DenyRedirects,
	}
}
