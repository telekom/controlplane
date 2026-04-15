// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package team

import "context"

// TeamDeps declares the narrow FK-resolution interface that the Team
// repository needs. This follows the Interface Segregation Principle (ISP):
// the repository depends only on the lookups it actually uses, not on the
// full IDResolver.
//
// The shared IDResolver satisfies this interface.
type TeamDeps interface {
	// FindGroupID resolves a Group name to its DB primary key.
	// Returns infrastructure.ErrEntityNotFound (wrapped) if the group
	// does not exist. The Team repository treats a missing Group as
	// non-fatal (optional FK).
	FindGroupID(ctx context.Context, name string) (int, error)
}
