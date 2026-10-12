// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"fmt"
	"net/url"

	"github.com/telekom/controlplane/common/pkg/util/urlutil"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

// RequireHTTPS rejects non-HTTPS URLs for custom-trust targets.
func RequireHTTPS(urls ...string) error {
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("parsing URL: %w", err)
		}
		if !urlutil.IsHTTPS(u) {
			return fmt.Errorf("URL %q must use https with a custom CA bundle", raw)
		}
	}
	return nil
}

// RequireSnapshotHTTPS rejects a persisted snapshot with a custom CA reference whose issuer,
// admin or token URL is not HTTPS. System-trust snapshots keep their existing behavior.
func RequireSnapshotHTTPS(s *identityv1.RealmStatus) error {
	if s.TLS.Ref() == nil {
		return nil
	}
	return RequireHTTPS(s.IssuerUrl, s.AdminUrl, s.AdminTokenUrl)
}
