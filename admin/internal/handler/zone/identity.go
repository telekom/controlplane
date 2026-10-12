// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"net/url"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/admin/internal/handler/util/urls"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

// prepareIdentityProvider stages the IdentityProvider configuration against the published provider
// without writing anything. It runs before the Zone CA bundle is
// written, so a rejected configuration changes neither the bundle nor the published provider.
func prepareIdentityProvider(ctx context.Context, hc *HandlingContext) error {
	hc.IDP = nil
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

	adminURL := ""
	if idp.Admin.Url != nil {
		adminURL = *idp.Admin.Url
	} else if idp.IssuerHostname != "" {
		adminURL = urls.ForIdentityProviderAdminUrl("https://" + idp.IssuerHostname)
	}

	existing, err := hc.getExisting(ctx, identityProvider)
	if err != nil {
		return err
	}
	var staged *identityapi.IdentityProvider
	if existing {
		if provErr := checkChildProvenance(hc, identityProvider); provErr != nil {
			return provErr
		}
		staged = identityProvider
	}

	prepared, err := newPreparedIdentityProvider(hc, idp, adminURL, staged)
	if err != nil {
		return err
	}
	hc.IDP = prepared
	return nil
}

// createIdentityProvider publishes the IdentityProvider prepared by prepareIdentityProvider.
func createIdentityProvider(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)
	prepared := hc.IDP
	if prepared == nil {
		return ctrlerrors.BlockedErrorf("identity provider of zone %q is not prepared", hc.Zone.Name)
	}
	idp := prepared.Config
	var staged client.Object
	if prepared.Staged != nil {
		staged = prepared.Staged
	}
	identityProvider := &identityapi.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForIdentityProvider(hc.Zone, idp.Name),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}
	if liveErr := hc.Handler.requireLive(ctx, hc.Zone); liveErr != nil {
		return liveErr
	}
	var guardErr error
	mutator := func() error {
		guardErr = stagedGuard(identityProvider, staged)
		if guardErr != nil {
			return guardErr
		}
		if identityProvider.Labels == nil {
			identityProvider.Labels = make(map[string]string)
		}
		identityProvider.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		identityProvider.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		identityProvider.Labels[cconfig.DomainLabelKey] = domainName
		setZoneUID(hc, identityProvider)
		identityProvider.Annotations[AnnotationDiscoveryIssuer] = prepared.DiscoveryIssuer

		identityProvider.Spec = identityapi.IdentityProviderSpec{
			AdminUrl:      prepared.AdminURL,
			AdminPassword: idp.Admin.Password,
			AdminClientId: idp.Admin.ClientId,
			AdminUserName: idp.Admin.UserName,
			TLS:           providerTLS(hc.Trust),
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, identityProvider, mutator)
	if guardErr != nil {
		return guardErr
	}
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update IdentityProvider %s in zone %s: %s", identityProvider.Name, hc.Zone.Name, err)
	}

	hc.IdentityProvider = identityProvider
	hc.Zone.Status.IdentityProvider = types.ObjectRefFromObject(identityProvider)
	return nil
}

func providerTLS(ts *trustSource) *types.ProviderTLS {
	if ts == nil {
		return nil
	}
	return &types.ProviderTLS{CABundleConfigMapRef: ts.PublishedRef()}
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
		SecretRotation: nil, // Internal realm MUST not have secret rotation enabled
	}
	realm, err := createIdentityRealm(ctx, hc, naming.ForInternalIdentityRealm(), opts)
	if err != nil {
		return err
	}

	hc.InternalIdentityRealm = realm
	hc.Zone.Status.InternalIdentityRealm = types.ObjectRefFromObject(realm)
	return nil
}

func issuerHostname(idp *adminv1.IdentityProviderConfig) (string, error) {
	if idp.IssuerHostname != "" {
		return idp.IssuerHostname, nil
	}
	if idp.Admin.Url == nil {
		return "", fmt.Errorf("identity provider %q has neither issuerHostname nor admin.url", idp.Name)
	}
	u, err := url.Parse(*idp.Admin.Url)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("identity provider %q has invalid admin.url", idp.Name)
	}
	return u.Hostname(), nil
}

// createIdentityRealmOptions configures the creation of an identity realm.
type createIdentityRealmOptions struct {
	Claims         []identityapi.ClaimConfig
	AllowedOrigins []string
	SecretRotation *adminv1.SecretRotationConfig
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

	var provenanceErr error
	mutator := func() error {
		if identityRealm.ResourceVersion != "" {
			provenanceErr = checkChildProvenance(hc, identityRealm)
			if provenanceErr != nil {
				return provenanceErr
			}
		}
		setZoneUID(hc, identityRealm)
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

	_, err := c.CreateOrUpdate(ctx, identityRealm, mutator)
	if provenanceErr != nil {
		return nil, provenanceErr
	}
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Identity Realm %s in zone %s: %s", identityRealm.Name, hc.Zone.Name, err)
	}
	return identityRealm, nil
}
