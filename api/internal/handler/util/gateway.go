// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"net/url"

	"github.com/pkg/errors"

	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// AsUpstream converts a raw URL to an Upstream struct, extracting the scheme, hostname, port, and path.
func AsUpstream(rawUrl string, weight int32) (ups gatewayapi.Upstream, err error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return ups, errors.Wrapf(err, "failed to parse URL %s", rawUrl)
	}

	port := gatewayapi.GetPortOrDefaultFromScheme(u)

	return gatewayapi.Upstream{
		Scheme:   u.Scheme,
		Hostname: u.Hostname(),
		Port:     port,
		Path:     u.Path,
		Weight:   weight,
	}, nil
}
