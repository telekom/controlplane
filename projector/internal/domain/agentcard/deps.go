// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard

import "context"

// AgentCardDeps declares the FK resolution interface required by the
// AgentCard repository. Team is a required dependency — if the owner Team is
// missing, the upsert fails with ErrDependencyMissing.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type AgentCardDeps interface {
	FindTeamID(ctx context.Context, name string) (int, error)
}
