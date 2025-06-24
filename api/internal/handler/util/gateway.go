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
	ups.ClientId = identityClient.Spec.ClientId
	ups.ClientSecret = identityClient.Spec.ClientSecret
	ups.IssuerUrl = identityClient.Status.IssuerUrl

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
		upstream.ClientId = apiExposure.Spec.ExternalIdp.ClientId
		upstream.ClientSecret = apiExposure.Spec.ExternalIdp.ClientSecret
		upstream.TokenEndpoint = apiExposure.Spec.ExternalIdp.TokenEndpoint
		upstream.TokenRequest = apiExposure.Spec.ExternalIdp.TokenEndpoint
	}

	return upstream, nil
}

func HasExternalIdp(exposure *apiapi.ApiExposure) bool {
	return exposure.Spec.ExternalIdp != apiapi.ExternalIdp{} || exposure.Spec.ExternalIdp.TokenEndpoint != ""
}
