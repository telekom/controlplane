// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import "context"

// FileTypeDeps declares the FK resolution interface required by the FileType
// repository. Team is a required dependency — if the owner Team is missing,
// the upsert fails with ErrDependencyMissing.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type FileTypeDeps interface {
	FindTeamID(ctx context.Context, name string) (int, error)
}
