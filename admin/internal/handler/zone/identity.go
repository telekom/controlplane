// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

		adminURL := ""
		if idp.Admin.Url != nil {
			adminURL = *idp.Admin.Url
		} else if idp.IssuerHostname != "" {
			adminURL = urls.ForIdentityProviderAdminUrl("https://" + idp.IssuerHostname)
		}

		identityProvider.Spec = identityapi.IdentityProviderSpec{
			AdminUrl:      adminURL,
			AdminPassword: idp.Admin.Password,
			AdminClientId: idp.Admin.ClientId,
			AdminUserName: idp.Admin.UserName,
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

func createGatewayAdminClient(ctx context.Context, hc *HandlingContext, gatewayConfig *adminv1.GatewayConfig) (*identityapi.Client, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	clientID := naming.ForGatewayAdminClientId()
	if gatewayConfig.Admin.ClientId != nil {
		clientID = *gatewayConfig.Admin.ClientId
	}

	adminClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForGatewayAdminClient(hc.IdentityProvider.Name),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	clientSecret := gatewayConfig.Admin.ClientSecret
	if clientSecret == nil {
		return nil, ctrlerrors.BlockedErrorf("gateway %q admin client secret must be provided for zone %q", gatewayConfig.Name, hc.Zone.Name)
	}

	mutator := func() error {
		if adminClient.Labels == nil {
			adminClient.Labels = make(map[string]string)
		}
		adminClient.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		adminClient.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		adminClient.Labels[cconfig.DomainLabelKey] = domainName

		adminClient.Spec = identityapi.ClientSpec{
			Realm:        types.ObjectRefFromObject(hc.InternalIdentityRealm),
			ClientId:     clientID,
			ClientSecret: *clientSecret,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, adminClient, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Gateway Admin Client %s in zone %s: %s", adminClient.Name, hc.Zone.Name, err)
	}

	return adminClient, nil
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
	SecretRotation *adminv1.SecretRotationConfig
}

// createIdentityRealm is a shared helper that creates an identity realm with the given name and options.
func createIdentityRealm(ctx context.Context, hc *HandlingContext, realmName string, opts createIdentityRealmOptions) (*identityapi.Realm, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	identityRealm := &identityapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(realmName),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
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
			Claims: opts.Claims,
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
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Identity Realm %s in zone %s: %s", identityRealm.Name, hc.Zone.Name, err)
	}
	return identityRealm, nil
}
