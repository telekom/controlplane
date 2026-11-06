// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"net/url"
	"sort"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

func populatePresetStatus(_ context.Context, hc *HandlingContext) error {
	statuses := make([]adminv1.PresetStatus, 0, len(hc.Zone.Spec.Presets))
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
		links, err := presetLinks(hc, preset)
		if err != nil {
			return err
		}
		tokenURL := preset.TokenUrl
		if tokenURL == "" {
			tokenURL = idp.TokenUrl
		}
		if tokenURL == "" {
			tokenURL, err = defaultRealmTokenURL(hc, preset.Name)
			if err != nil {
				return err
			}
		}
		links.TokenUrl = tokenURL
		statuses = append(statuses, adminv1.PresetStatus{
			Name: preset.Name, GatewayRef: gatewayStatus.Gateway, IdentityProviderRef: hc.Zone.Status.IdentityProvider, Links: links,
		})
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	hc.Zone.Status.Presets = statuses
	return nil
}

func presetLinks(hc *HandlingContext, preset *adminv1.Preset) (adminv1.Links, error) {
	if hc.DefaultIdentityRealm == nil {
		return adminv1.Links{}, ctrlerrors.RetryableErrorf("cannot build links for preset %q: default identity realm is not resolved", preset.Name)
	}
	issuer, err := realmIssuerURL(hc.DefaultIdentityRealm)
	if err != nil {
		return adminv1.Links{}, err
	}
	internalIssuer, err := internalIssuerURL(hc)
	if err != nil {
		return adminv1.Links{}, err
	}
	baseURL := preset.GetDefaultURL()
	lmsBase := baseURL
	if hc.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld {
		lmsBase += spacegatePathPrefix
	}
	lmsIssuer, err := url.JoinPath(lmsBase, "auth/realms", hc.DefaultIdentityRealm.Name)
	if err != nil {
		return adminv1.Links{}, ctrlerrors.BlockedErrorf("cannot build LMS issuer for preset %q: %s", preset.Name, err)
	}
	links := adminv1.Links{Url: baseURL, Issuer: issuer, LmsIssuer: lmsIssuer, InternalIssuer: internalIssuer}
	if hc.TeamApiIdentityRealm != nil {
		links.TeamIssuer, err = realmIssuerURL(hc.TeamApiIdentityRealm)
		if err != nil {
			return adminv1.Links{}, err
		}
	}
	if cconfig.FeaturePermission.IsEnabled() && hc.Zone.Spec.Permissions != nil {
		links.PermissionsUrl, err = url.JoinPath(baseURL, hc.Zone.Spec.Permissions.ApiBasePath)
		if err != nil {
			return adminv1.Links{}, ctrlerrors.BlockedErrorf("cannot build permissions URL for preset %q: %s", preset.Name, err)
		}
		hc.Zone.EnableFeature(adminv1.FeaturePermissions)
	} else {
		hc.Zone.ManageFeature(adminv1.FeaturePermissions, false)
	}
	return links, nil
}
