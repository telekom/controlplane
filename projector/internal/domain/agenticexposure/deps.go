// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import "context"

// AgenticExposureDeps declares the FK resolution interface required by the
// AgenticExposure repository. Application is a required dependency — if the
// owner Application is missing, the upsert fails with ErrDependencyMissing.
// McpServer/AgentCard are optional and mutually exclusive (selected by
// Variant) — if no active entry exists for the base path, the corresponding
// FK is left null.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type AgenticExposureDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindActiveMcpServerID(ctx context.Context, basePath string) (int, error)
	FindActiveAgentCardID(ctx context.Context, basePath string) (int, error)
}
