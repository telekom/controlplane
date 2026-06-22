// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"math"
	"net/url"

	"github.com/pkg/errors"

	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// AsUpstreamForRealRoute parses the given URL and returns an Upstream pointing at it.
func AsUpstreamForRealRoute(rawUrl string, weight int32) (ups gatewayapi.Upstream, err error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return ups, errors.Wrapf(err, "failed to parse URL %s", rawUrl)
	}

	port := gatewayapi.GetPortOrDefaultFromScheme(u)
	if port < 0 || port > math.MaxInt32 {
		return ups, errors.Errorf("port %d out of int32 range for URL %s", port, rawUrl)
	}

	return gatewayapi.Upstream{
		Scheme:   u.Scheme,
		Hostname: u.Hostname(),
		Port:     int32(port),
		Path:     u.Path,
		Weight:   weight,
	}, nil
}
