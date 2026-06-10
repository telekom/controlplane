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

	// Gateway base URL
	zone.Status.Links.Url = zone.Spec.Gateway.Url

	// Issuer URL: <idpUrl>/auth/realms/<realmName>
	issuer, err := url.JoinPath(zone.Spec.IdentityProvider.Url, "auth/realms/", hc.DefaultIdentityRealm.Name)
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot combine identityProviderBaseUrl %s with realm name %s: %s", zone.Spec.IdentityProvider.Url, hc.DefaultIdentityRealm.Name, err)
	}
	zone.Status.Links.Issuer = issuer

	// LMS Issuer URL: <gatewayUrl>/auth/realms/<realmName>
	lmsIssuer, err := url.JoinPath(zone.Spec.Gateway.Url, "auth/realms/", hc.DefaultGatewayRealm.Name)
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot combine gatewayUrl %s with realm name %s: %s", zone.Spec.Gateway.Url, hc.DefaultGatewayRealm.Name, err)
	}
	zone.Status.Links.LmsIssuer = lmsIssuer

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

	return nil
}
