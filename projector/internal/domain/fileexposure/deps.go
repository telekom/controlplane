// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure

import "context"

// FileExposureDeps declares the FK resolution interfaces required by the
// FileExposure repository.
type FileExposureDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindZoneID(ctx context.Context, name string) (int, error)
	FindFileTypeID(ctx context.Context, fileType string) (int, error)
}
