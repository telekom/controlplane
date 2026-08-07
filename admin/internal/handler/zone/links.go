// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

func populatePresetStatus(ctx context.Context, hc *HandlingContext) error {
	statuses := make([]adminv1.PresetStatus, 0, len(hc.Zone.Spec.Presets))
	tokenURLs := make(map[string]string, len(hc.Zone.Spec.Presets))
	previousStatuses := make(map[string]adminv1.PresetStatus, len(hc.Zone.Status.Presets))
	for _, status := range hc.Zone.Status.Presets {
		previousStatuses[status.Name] = status
	}
	for i := range hc.Zone.Spec.Presets {
		preset := &hc.Zone.Spec.Presets[i]
		gateway, err := hc.Zone.Spec.GetGateway(preset.GatewayRef)
		if err != nil {
			return ctrlerrors.BlockedErrorf("cannot resolve gateway for preset %q: %s", preset.Name, err)
		}
		idp, err := hc.Zone.Spec.GetIdentityProviderByName(preset.IdentityProviderRef)
		if err != nil {
			return ctrlerrors.BlockedErrorf("cannot resolve identity provider for preset %q: %s", preset.Name, err)
		}
		gatewayStatus, err := hc.Zone.Status.GetGateway(gateway.Name)
		if err != nil {
			return ctrlerrors.BlockedErrorf("cannot resolve gateway status for preset %q: %s", preset.Name, err)
		}
		links, issuer, err := presetLinks(hc, preset, idp)
		if err != nil {
			return err
		}
		tokenURL := preset.TokenUrl
		if tokenURL == "" {
			tokenURL = idp.TokenUrl
		}
		if tokenURL == "" {
			tokenURL = tokenURLs[issuer]
			previous := previousStatuses[preset.Name]
			if tokenURL == "" && previous.Links.Issuer == issuer && validateDiscoveredTokenURL(previous.TokenUrl) == nil {
				tokenURL = previous.TokenUrl
				tokenURLs[issuer] = tokenURL
			}
			if tokenURL == "" {
				tokenURL, err = discoverTokenURL(ctx, hc.HTTPClient, issuer)
				if err != nil {
					return ctrlerrors.RetryableErrorf("failed OIDC discovery for preset %q: %s", preset.Name, err)
				}
				tokenURLs[issuer] = tokenURL
			}
		}
		statuses = append(statuses, adminv1.PresetStatus{
			Name: preset.Name, GatewayRef: gatewayStatus.Gateway, IdentityProviderRef: hc.Zone.Status.IdentityProvider, Links: links, TokenUrl: tokenURL,
		})
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	hc.Zone.Status.Presets = statuses
	return nil
}

func presetLinks(hc *HandlingContext, preset *adminv1.Preset, idp *adminv1.IdentityProviderConfig) (adminv1.Links, string, error) {
	hostname, err := issuerHostname(idp)
	if err != nil {
		return adminv1.Links{}, "", ctrlerrors.BlockedErrorf("cannot resolve issuer hostname for preset %q: %s", preset.Name, err)
	}
	issuer := fmt.Sprintf("https://%s/auth/realms/%s", hostname, hc.DefaultIdentityRealm.Name)
	internalIssuer := fmt.Sprintf("https://%s/auth/realms/%s", hostname, hc.InternalIdentityRealm.Name)
	baseURL := preset.GetDefaultURL()
	lmsBase := baseURL
	if hc.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld {
		lmsBase += spacegatePathPrefix
	}
	lmsIssuer, err := url.JoinPath(lmsBase, "auth/realms", hc.DefaultIdentityRealm.Name)
	if err != nil {
		return adminv1.Links{}, "", ctrlerrors.BlockedErrorf("cannot build LMS issuer for preset %q: %s", preset.Name, err)
	}
	links := adminv1.Links{Url: baseURL, Issuer: issuer, LmsIssuer: lmsIssuer, InternalIssuer: internalIssuer}
	if hc.TeamApiIdentityRealm != nil {
		links.TeamIssuer = fmt.Sprintf("https://%s/auth/realms/%s", hostname, hc.TeamApiIdentityRealm.Name)
	}
	if cconfig.FeaturePermission.IsEnabled() && hc.Zone.Spec.Permissions != nil {
		links.PermissionsUrl, err = url.JoinPath(baseURL, hc.Zone.Spec.Permissions.ApiBasePath)
		if err != nil {
			return adminv1.Links{}, "", ctrlerrors.BlockedErrorf("cannot build permissions URL for preset %q: %s", preset.Name, err)
		}
		hc.Zone.EnableFeature(adminv1.FeaturePermissions)
	} else {
		hc.Zone.ManageFeature(adminv1.FeaturePermissions, false)
	}
	return links, issuer, nil
}
