// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"net/url"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

// populateLinks computes all status links from the zone spec and created resources.
func populateLinks(ctx context.Context, hc *HandlingContext) error {
	zone := hc.Zone

	defaultPreset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot resolve default gateway preset for links: %s", err)
	}

	// Gateway base URL (from default preset)
	zone.Status.Links.Url = defaultPreset.GetDefaultUrl()

	// Issuer URL: <idpUrl>/auth/realms/<realmName>
	issuer, err := url.JoinPath(zone.Spec.IdentityProvider.Url, "auth/realms/", hc.DefaultIdentityRealm.Name)
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot combine identityProviderBaseUrl %s with realm name %s: %s", zone.Spec.IdentityProvider.Url, hc.DefaultIdentityRealm.Name, err)
	}
	zone.Status.Links.Issuer = issuer

	// LMS Issuer URL: <gatewayPresetUrl>/auth/realms/<identityRealmName>
	// or for Spacegate: <gatewayPresetUrl>/spacegate/auth/realms/<identityRealmName>
	pathPrefix := defaultPreset.GetDefaultUrl()
	if hc.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld {
		pathPrefix = defaultPreset.GetDefaultUrl() + spacegatePathPrefix
	}
	lmsIssuer, err := url.JoinPath(pathPrefix, "auth/realms/", hc.DefaultIdentityRealm.Name)
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot combine gateway preset URL with realm name %s: %s", hc.DefaultIdentityRealm.Name, err)
	}
	zone.Status.Links.LmsIssuer = lmsIssuer

	internalIssuer, err := url.JoinPath(zone.Spec.IdentityProvider.Url, "auth/realms/", hc.InternalIdentityRealm.Name)
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot combine identityProviderBaseUrl %s with realm name %s: %s", zone.Spec.IdentityProvider.Url, hc.InternalIdentityRealm.Name, err)
	}
	zone.Status.Links.InternalIssuer = internalIssuer

	// Permissions URL (if configured and feature enabled)
	if cconfig.FeaturePermission.IsEnabled() && zone.Spec.Permissions != nil {
		permissionsUrl, err := url.JoinPath(zone.Status.Links.Url, zone.Spec.Permissions.ApiBasePath)
		if err != nil {
			return ctrlerrors.BlockedErrorf("failed to build permissions URL: %s", err)
		}
		zone.Status.Links.PermissionsUrl = permissionsUrl
		zone.EnableFeature(adminv1.FeaturePermissions)
	} else {
		zone.Status.Links.PermissionsUrl = ""
		zone.ManageFeature(adminv1.FeaturePermissions, false)
	}

	_, presetErr := adminv1.SelectGatewayPreset(hc.Zone.Spec.Gateway.Presets, adminv1.FeatureConsumerFailover)
	if presetErr == nil {
		zone.EnableFeature(adminv1.FeatureConsumerFailover)
	} else {
		zone.ManageFeature(adminv1.FeatureConsumerFailover, false)
	}

	return nil
}
