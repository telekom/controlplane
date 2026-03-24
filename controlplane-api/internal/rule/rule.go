// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rule

import (
	"context"

	"entgo.io/ent/privacy"

	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

// DenyIfNoViewer denies access if no viewer is present in the context.
func DenyIfNoViewer() privacy.QueryMutationRule {
	return privacy.ContextQueryMutationRule(func(ctx context.Context) error {
		v := viewer.FromContext(ctx)
		if v == nil {
			return privacy.Denyf("viewer-context is missing")
		}
		return privacy.Skip
	})
}

// DenyIfNoTeams denies access if the viewer has no team memberships and is not an admin.
func DenyIfNoTeams() privacy.QueryMutationRule {
	return privacy.ContextQueryMutationRule(func(ctx context.Context) error {
		v := viewer.FromContext(ctx)
		if v == nil {
			return privacy.Denyf("viewer-context is missing")
		}
		if v.Admin {
			return privacy.Skip
		}
		if len(v.Teams) == 0 {
			return privacy.Denyf("viewer has no team access")
		}
		return privacy.Skip
	})
}
