// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"

	secretsapi "github.com/telekom/controlplane/secret-manager/api"
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

	identityProvider := &identityapi.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForIdentityProvider(hc.Zone)),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	mutator := func() error {
		if identityProvider.Labels == nil {
			identityProvider.Labels = make(map[string]string)
		}
		identityProvider.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		identityProvider.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name

		adminUrl := urls.ForIdentityProviderAdminUrl(hc.Zone.Spec.IdentityProvider.Url)
		if hc.Zone.Spec.IdentityProvider.Admin.Url != nil {
			adminUrl = *hc.Zone.Spec.IdentityProvider.Admin.Url
		}

		identityProvider.Spec = identityapi.IdentityProviderSpec{
			AdminUrl:      adminUrl,
			AdminPassword: hc.Zone.Spec.IdentityProvider.Admin.Password,
			AdminClientId: hc.Zone.Spec.IdentityProvider.Admin.ClientId,
			AdminUserName: hc.Zone.Spec.IdentityProvider.Admin.UserName,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, identityProvider, mutator)
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update IdentityProvider %s in zone %s: %s", identityProvider.Name, hc.Zone.Name, err)
	}

	hc.IdentityProvider = identityProvider
	hc.Zone.Status.IdentityProvider = types.ObjectRefFromObject(identityProvider)
	return nil
}

// createDefaultIdentityRealm creates the default identity realm for the zone.
func createDefaultIdentityRealm(ctx context.Context, hc *HandlingContext) error {
	opts := createIdentityRealmOptions{
		Claims:         hc.DefaultClaims,
		SecretRotation: hc.Zone.Spec.IdentityProvider.SecretRotation,
	}
	realm, err := createIdentityRealm(ctx, hc, naming.ForDefaultIdentityRealm(hc.Environment), opts)
	if err != nil {
		return err
	}

	hc.DefaultIdentityRealm = realm
	hc.Zone.Status.IdentityRealm = types.ObjectRefFromObject(realm)
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

// createGatewayAdminClient creates the "rover" client in the internal identity realm
// for gateway admin API authentication. This step is a no-op when the gateway admin
// is externally managed (i.e., the user provides clientId/clientSecret/tokenUrl).
func createGatewayAdminClient(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)

	adminClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForGatewayAdminClientId()),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	clientSecret := hc.Zone.Spec.Gateway.Admin.ClientSecret
	if clientSecret == nil {
		return ctrlerrors.BlockedErrorf("gateway admin client secret must be provided for zone %q", hc.Zone.Name)
	}

	mutator := func() error {
		if adminClient.Labels == nil {
			adminClient.Labels = make(map[string]string)
		}
		adminClient.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		adminClient.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name

		adminClient.Spec = identityapi.ClientSpec{
			Realm:        types.ObjectRefFromObject(hc.InternalIdentityRealm),
			ClientId:     naming.ForGatewayAdminClientId(),
			ClientSecret: *clientSecret,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, adminClient, mutator)
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update Gateway Admin Client %s in zone %s: %s", adminClient.Name, hc.Zone.Name, err)
	}

	hc.GatewayAdminClient = adminClient
	hc.Zone.Status.GatewayAdminClient = types.ObjectRefFromObject(adminClient)
	return nil
}

// createGatewayClient creates the gateway's OAuth2 client in the default identity realm.
// TODO: This will be removed when the transition to new the mesh-auth logic is complete.
func createGatewayClient(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)

	identityClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForGatewayClient()),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	mutator := func() error {
		if identityClient.Labels == nil {
			identityClient.Labels = make(map[string]string)
		}
		identityClient.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		identityClient.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name

		// Assumption is that this will be replaced by the new mesh-auth logic before release,
		// so we can leave the plain-text secret here for now.
		clientSecret := identityClient.Spec.ClientSecret
		if clientSecret == "" {
			var err error
			clientSecret, err = secretsapi.GenerateSecret()
			if err != nil {
				return fmt.Errorf("failed to generate client secret for Gateway Client in zone %s: %w", hc.Zone.Name, err)
			}
		}

		identityClient.Spec = identityapi.ClientSpec{
			Realm:        types.ObjectRefFromObject(hc.DefaultIdentityRealm),
			ClientId:     naming.ForGatewayClient(),
			ClientSecret: clientSecret,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, identityClient, mutator)
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update Identity Client %s in zone %s: %s", identityClient.Name, hc.Zone.Name, err)
	}

	hc.GatewayClient = identityClient
	hc.Zone.Status.GatewayClient = types.ObjectRefFromObject(identityClient)
	return nil
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

			hc.Zone.EnableFeature(adminv1.FeatureSecretRotation)
		} else {
			identityRealm.Spec.SecretRotation = nil
			hc.Zone.ManageFeature(adminv1.FeatureSecretRotation, false)
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, identityRealm, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Identity Realm %s in zone %s: %s", identityRealm.Name, hc.Zone.Name, err)
	}
	return identityRealm, nil
}

// getIdentityClient retrieves an existing identity client by reference.
func getIdentityClient(ctx context.Context, ref *types.ObjectRef) (*identityapi.Client, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	identityClient := &identityapi.Client{}
	err := c.Get(ctx, ref.K8s(), identityClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity client %s: %w", ref.Name, err)
	}
	return identityClient, nil
}
