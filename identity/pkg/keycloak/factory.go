// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"github.com/telekom/controlplane/common-server/pkg/client/tlsclient"
	"github.com/telekom/controlplane/common/pkg/cabundle"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

const (
	profileName    = "identity"
	realmOwnerKind = "identity.cp.ei.telekom.de/Realm"
	idpOwnerKind   = "identity.cp.ei.telekom.de/IdentityProvider"
	defaultTimeout = 10 * time.Second
	clientName     = "identity"
)

// ServiceFactory creates KeycloakService instances for the admin connection
// described by a RealmStatus of a Realm.
type ServiceFactory interface {
	// ServiceFor returns a service for status, which belongs to realm and may
	// carry resolved secrets, and a function releasing it. The release
	// function must be called once the service is no longer used.
	ServiceFor(ctx context.Context, realm *identityv1.Realm, status identityv1.RealmStatus) (KeycloakService, func(), error)
}

// ServiceFactoryFunc adapts a function with the ServiceFor signature to the
// ServiceFactory interface. Useful for tests.
type ServiceFactoryFunc func(ctx context.Context, realm *identityv1.Realm, status identityv1.RealmStatus) (KeycloakService, func(), error)

// ServiceFor implements ServiceFactory by calling f.
//
//nolint:gocritic // ServiceFactory keeps a value-based status for callers.
func (f ServiceFactoryFunc) ServiceFor(ctx context.Context, realm *identityv1.Realm, status identityv1.RealmStatus) (KeycloakService, func(), error) {
	return f(ctx, realm, status)
}

// configError marks an invalid connection configuration. It satisfies the
// ctrlerrors.BlockedError interface via duck-typing.
type configError struct {
	err error
}

func (e *configError) Error() string {
	return e.err.Error()
}

func (e *configError) Unwrap() error {
	return e.err
}

// IsBlocked satisfies the ctrlerrors.BlockedError interface.
func (e *configError) IsBlocked() bool {
	return true
}

func connectionErrorf(format string, a ...any) error {
	return &configError{err: fmt.Errorf(format, a...)}
}

// CachedServiceFactory is the production ServiceFactory. A single instance
// is shared by all controllers of the operator and must be added to the
// manager to clean up idle clients.
type CachedServiceFactory struct {
	clients *tlsclient.Factory
	reader  client.Reader
}

var _ ServiceFactory = &CachedServiceFactory{}

// NewServiceFactory creates the factory. ctx is the operator lifetime and
// reader is used to read CA bundle ConfigMaps (the manager cached client).
// The HTTP timeout defaults to 10s and can be overridden via CLIENT_TIMEOUT.
func NewServiceFactory(ctx context.Context, reader client.Reader, opts ...tlsclient.FactoryOption) (*CachedServiceFactory, error) {
	p, err := profile(commonclient.EnvClientTimeout)
	if err != nil {
		return nil, err
	}
	clients, err := tlsclient.NewFactory(ctx, []tlsclient.Profile{p}, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating keycloak client factory: %w", err)
	}
	return &CachedServiceFactory{clients: clients, reader: reader}, nil
}

func profile(rawTimeout string) (tlsclient.Profile, error) {
	timeout := defaultTimeout
	if rawTimeout != "" {
		var err error
		timeout, err = time.ParseDuration(rawTimeout)
		if err != nil {
			return tlsclient.Profile{}, fmt.Errorf("invalid CLIENT_TIMEOUT value: %w", err)
		}
	}
	return tlsclient.Profile{
		Name:          profileName,
		Transport:     tlsclient.TransportRaw,
		Timeout:       timeout,
		MinTLSVersion: tls.VersionTLS13,
		ClientName:    clientName,
		MetricsReplacePatterns: []string{
			`([^\/]+--[^\/]+--[^\/]+)`, `([^\/]+--[^\/]+)`, metrics.ReplacePatternUID,
		},
		ClassifyTokenError: classifyOAuth2Error,
	}, nil
}

// Start runs the cleanup of idle clients. It implements manager.Runnable.
func (f *CachedServiceFactory) Start(ctx context.Context) error {
	return f.clients.Start(ctx)
}

// NeedLeaderElection lets the cleanup run on every replica.
func (f *CachedServiceFactory) NeedLeaderElection() bool {
	return false
}

// ServiceFor implements ServiceFactory. Clients are scoped to the
// IdentityProvider recorded in status.Connection, or to the Realm for
// legacy statuses without connection metadata (system roots only).
//
//nolint:gocritic // ServiceFactory keeps a value-based status for callers.
func (f *CachedServiceFactory) ServiceFor(ctx context.Context, realm *identityv1.Realm, status identityv1.RealmStatus) (KeycloakService, func(), error) {
	lease, err := f.acquire(ctx, realm, &status)
	if err != nil {
		return nil, nil, err
	}
	apiClient, err := api.NewClientWithResponses(lease.Client().APIURL(), api.WithHTTPClient(lease.Client().Doer()))
	if err != nil {
		lease.Release()
		return nil, nil, fmt.Errorf("failed to create keycloak admin client: %w", err)
	}
	return NewKeycloakService(apiClient,
		WithIssuerDiscovery(lease.Client().UnauthenticatedDoer(), lease.Client().APIURL())), lease.Release, nil
}

func (f *CachedServiceFactory) acquire(ctx context.Context, realm *identityv1.Realm, status *identityv1.RealmStatus) (*tlsclient.Lease, error) {
	if realm == nil {
		return nil, errors.New("realm is nil")
	}
	spec := tlsclient.Spec{
		Profile: profileName,
		APIURL:  status.AdminUrl,
		Auth: tlsclient.Auth{
			Mode:     tlsclient.AuthPassword,
			TokenURL: status.AdminTokenUrl,
			ClientID: status.AdminClientId,
			Username: status.AdminUserName,
			Password: status.AdminPassword,
		},
	}
	if err := f.applyConnection(&spec, realm, status.Connection); err != nil {
		return nil, err
	}

	lease, err := f.clients.Acquire(ctx, spec)
	if err != nil {
		return nil, classifyAcquireError(err)
	}
	return lease, nil
}

// applyConnection sets owner and CA of spec from the connection metadata.
func (f *CachedServiceFactory) applyConnection(spec *tlsclient.Spec, realm *identityv1.Realm, conn *identityv1.RealmConnection) error {
	if conn == nil {
		spec.Owner = tlsclient.Owner{Kind: realmOwnerKind, Namespace: realm.Namespace, Name: realm.Name, UID: string(realm.UID)}
		return nil
	}
	if err := validateConnection(conn); err != nil {
		return connectionErrorf("realm %s/%s has incomplete connection metadata: %w", realm.Namespace, realm.Name, err)
	}
	idp := conn.IdentityProvider
	spec.Owner = tlsclient.Owner{Kind: idpOwnerKind, Namespace: idp.Namespace, Name: idp.Name, UID: string(idp.UID)}
	if conn.TLS == nil {
		return nil
	}
	if f.reader == nil {
		return fmt.Errorf("realm %s/%s references a CA bundle but no resolver is configured", realm.Namespace, realm.Name)
	}
	spec.CA = cabundle.NewResolver(f.reader, conn.TLS.Namespace, conn.TLS.ConfigMapRef)
	return nil
}

func validateConnection(conn *identityv1.RealmConnection) error {
	idp := conn.IdentityProvider
	if idp.Name == "" || idp.Namespace == "" || idp.UID == "" {
		return errors.New("identity provider name, namespace and uid are required")
	}
	if conn.TLS != nil && (conn.TLS.Namespace == "" || conn.TLS.ConfigMapRef.Name == "" || conn.TLS.ConfigMapRef.Key == "") {
		return errors.New("tls namespace, configmap name and key are required")
	}
	return nil
}

// classifyAcquireError blocks on invalid CA configuration. Other errors are
// returned unchanged and retried.
func classifyAcquireError(err error) error {
	if errors.Is(err, cabundle.ErrInvalidBundle) ||
		errors.Is(err, cabundle.ErrSourceNotFound) ||
		errors.Is(err, cabundle.ErrSourceForbidden) ||
		errors.Is(err, tlsclient.ErrInvalidCA) {
		return connectionErrorf("invalid CA configuration: %w", err)
	}
	return err
}
