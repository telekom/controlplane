// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"net/url"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

// createIdentityProvider creates the IdentityProvider resource for the zone.
func createIdentityProvider(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)

	idp, err := hc.Zone.Spec.GetIdentityProvider()
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot resolve identity provider for zone %q: %s", hc.Zone.Name, err)
	}
	identityProvider := &identityapi.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForIdentityProvider(hc.Zone, idp.Name),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	mutator := func() error {
		if identityProvider.Labels == nil {
			identityProvider.Labels = make(map[string]string)
		}
		identityProvider.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		identityProvider.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		identityProvider.Labels[cconfig.DomainLabelKey] = domainName

		if idp.Admin.Url == "" {
			return ctrlerrors.BlockedErrorf("identity provider %q has no admin.url", idp.Name)
		}

		identityProvider.Spec = identityapi.IdentityProviderSpec{
			AdminUrl:      idp.Admin.Url,
			AdminPassword: idp.Admin.Password,
			AdminClientId: idp.Admin.ClientId,
			AdminUserName: idp.Admin.UserName,
			TLS:           hc.managedTLS(),
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, identityProvider, mutator)
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update IdentityProvider %s in zone %s: %s", identityProvider.Name, hc.Zone.Name, err)
	}

	hc.IdentityProvider = identityProvider
	hc.Zone.Status.IdentityProvider = types.ObjectRefFromObject(identityProvider)
	return nil
}

// createDefaultIdentityRealm creates the default identity realm for the zone.
func createDefaultIdentityRealm(ctx context.Context, hc *HandlingContext) error {
	idp, err := hc.Zone.Spec.GetIdentityProvider()
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot resolve identity provider for zone %q: %s", hc.Zone.Name, err)
	}
	opts := createIdentityRealmOptions{
		Claims:         hc.DefaultClaims,
		AllowedOrigins: slices.Clone(idp.AllowedOrigins),
		SecretRotation: idp.SecretRotation,
		SetFrontendURL: true,
	}
	realm, err := createIdentityRealm(ctx, hc, naming.ForDefaultIdentityRealm(hc.Environment), opts)
	if err != nil {
		return err
	}

	hc.DefaultIdentityRealm = realm
	hc.Zone.Status.IdentityRealm = types.ObjectRefFromObject(realm)

	if realm.Spec.SecretRotation != nil {
		hc.Zone.EnableFeature(adminv1.FeatureSecretRotation)
	} else {
		hc.Zone.ManageFeature(adminv1.FeatureSecretRotation, false)
	}

	return nil
}

// createInternalIdentityRealm creates the internal "rover" realm for admin-config clients.
func createInternalIdentityRealm(ctx context.Context, hc *HandlingContext) error {
	opts := createIdentityRealmOptions{
		Claims:         hc.DefaultClaims,
		SecretRotation: nil,   // Internal realm MUST not have secret rotation enabled
		SetFrontendURL: false, // Internal (rover) realm MUST not have a frontendUrl
	}
	realm, err := createIdentityRealm(ctx, hc, naming.ForInternalIdentityRealm(), opts)
	if err != nil {
		return err
	}

	hc.InternalIdentityRealm = realm
	hc.Zone.Status.InternalIdentityRealm = types.ObjectRefFromObject(realm)
	return nil
}

// internalIssuerURL returns the issuer URL the identity operator resolved for the
// internal (rover) realm.
func internalIssuerURL(hc *HandlingContext) (string, error) {
	return realmIssuerURL(hc.InternalIdentityRealm)
}

// realmIssuerURL returns the issuer URL the identity operator resolved for the
// realm. Public links and trusted issuers must use it instead of building one
// from a hostname. A trailing "/" is trimmed for consumers that append path suffixes.
func realmIssuerURL(realm *identityapi.Realm) (string, error) {
	issuer := realm.Status.IssuerUrl
	if issuer == "" {
		return "", ctrlerrors.RetryableErrorf("identity realm %q has no resolved issuer URL yet", realm.Name)
	}
	return strings.TrimSuffix(issuer, "/"), nil
}

// keycloakFrontendURLAttribute is the Keycloak realm attribute overriding the realm base URL.
const keycloakFrontendURLAttribute = "frontendUrl"

// realmFrontendURL returns the Keycloak frontend (base) URL for the issuer
// hostname, e.g. "https://iam.example.com/auth".
func realmFrontendURL(hostname string) string {
	return (&url.URL{Scheme: "https", Host: hostname, Path: "/auth"}).String()
}

// createIdentityRealmOptions configures the creation of an identity realm.
type createIdentityRealmOptions struct {
	Claims         []identityapi.ClaimConfig
	AllowedOrigins []string
	SecretRotation *adminv1.SecretRotationConfig
	// SetFrontendURL sets the realm frontendUrl attribute on World zones that
	// configure an explicit issuerHostname. It must be false for the internal (rover) realm.
	SetFrontendURL bool
}

// createIdentityRealm is a shared helper that creates an identity realm with the given name and options.
func createIdentityRealm(ctx context.Context, hc *HandlingContext, realmName string, opts createIdentityRealmOptions) (*identityapi.Realm, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	identityRealm := &identityapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(realmName),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	idp, err := hc.Zone.Spec.GetIdentityProvider()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("cannot resolve identity provider for zone %q: %s", hc.Zone.Name, err)
	}
	// Without an explicit issuerHostname the frontendUrl stays unset, so the identity
	// operator removes a previously managed value and resolves the issuer through discovery.
	frontendURL := ""
	if opts.SetFrontendURL && hc.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld && idp.IssuerHostname != "" {
		frontendURL = realmFrontendURL(idp.IssuerHostname)
	}

	mutator := func() error {
		if identityRealm.Labels == nil {
			identityRealm.Labels = make(map[string]string)
		}
		identityRealm.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		identityRealm.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		identityRealm.Labels[cconfig.DomainLabelKey] = domainName

		identityRealm.Spec = identityapi.RealmSpec{
			IdentityProvider: &types.ObjectRef{
				Name:      hc.IdentityProvider.Name,
				Namespace: hc.IdentityProvider.Namespace,
			},
			Claims:         opts.Claims,
			AllowedOrigins: opts.AllowedOrigins,
		}

		if frontendURL != "" {
			identityRealm.Spec.Attributes = map[string]string{
				keycloakFrontendURLAttribute: frontendURL,
			}
		}

		secretRotationConfig := opts.SecretRotation
		if secretRotationConfig != nil && secretRotationConfig.Enabled {
			identityRealm.Spec.SecretRotation = &identityapi.SecretRotationConfig{
				GracePeriod:             secretRotationConfig.GracePeriod,
				ExpirationPeriod:        secretRotationConfig.ExpirationPeriod,
				RemainingRotationPeriod: secretRotationConfig.ExpirationPeriod,
			}
		} else {
			identityRealm.Spec.SecretRotation = nil
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, identityRealm, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Identity Realm %s in zone %s: %s", identityRealm.Name, hc.Zone.Name, err)
	}
	return identityRealm, nil
}
