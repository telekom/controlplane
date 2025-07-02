// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AsUpstreamForProxyRoute(ctx context.Context, realm *gatewayapi.Realm, apiBasePath string) (ups gatewayapi.Upstream, err error) {
	c := cclient.ClientFromContextOrDie(ctx)

	identityClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: realm.Namespace,
		},
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(identityClient), identityClient); err != nil {
		return ups, errors.Wrapf(err, "failed to get gateway client for %s", realm.Namespace)
	}

	ups, err = realm.AsUpstream(apiBasePath)
	if err != nil {
		return ups, errors.Wrapf(err, "failed to construct upstream for realm %s", realm.Name)
	}

	ups.IssuerUrl = identityClient.Status.IssuerUrl
	ups.Security = &gatewayapi.Security{
		M2M: &gatewayapi.Machine2MachineAuthentication{
			Client: gatewayapi.OAuth2ClientCredentials{
				ClientId:     identityClient.Spec.ClientId,
				ClientSecret: identityClient.Spec.ClientSecret,
			},
		},
	}

	return
}

func AsUpstreamForRealRoute(
	ctx context.Context, apiExposure *apiapi.ApiExposure) (ups gatewayapi.Upstream, err error) {

	rawUrl := apiExposure.Spec.Upstreams[0].Url
	url, err := url.Parse(rawUrl)
	if err != nil {
		return ups, errors.Wrapf(err, "failed to parse URL %s", rawUrl)
	}

	upstream := gatewayapi.Upstream{
		Scheme: url.Scheme,
		Host:   url.Hostname(),
		Port:   gatewayapi.GetPortOrDefaultFromScheme(url),
		Path:   url.Path,
	}

	if HasExternalIdp(apiExposure) {
		// client
		// ToDo Refactor
		upstream.Security = &gatewayapi.Security{
			M2M: &gatewayapi.Machine2MachineAuthentication{
				ExternalIDPConfig: &gatewayapi.ExternalIdentityProviderConfig{
					TokenEndpoint: apiExposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint,
					TokenRequest:  apiExposure.Spec.Security.M2M.ExternalIDP.TokenRequest,
					GrantType:     apiExposure.Spec.Security.M2M.ExternalIDP.GrantType,
				},
				Client: gatewayapi.OAuth2ClientCredentials{
					ClientId:     apiExposure.Spec.Security.M2M.ExternalIDP.Client.ClientId,
					ClientSecret: apiExposure.Spec.Security.M2M.ExternalIDP.Client.ClientSecret,
					Scopes:       apiExposure.Spec.Security.M2M.ExternalIDP.Client.Scopes,
				},
			},
		}
	}

	return upstream, nil
}

func HasExternalIdp(exposure *apiapi.ApiExposure) bool {

	if exposure.Spec.Security == nil {
		return false
	}
	if exposure.Spec.Security.M2M == nil {
		return false
	}
	if exposure.Spec.Security.M2M.ExternalIDP == nil {
		return false
	}

	return exposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint != ""
}
