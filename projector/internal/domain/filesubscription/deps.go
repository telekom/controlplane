// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import "context"

// FileSubscriptionDeps declares the FK resolution interfaces required by the
// FileSubscription repository.
type FileSubscriptionDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindZoneID(ctx context.Context, name string) (int, error)
	FindFileTypeID(ctx context.Context, fileType string) (int, error)
	FindActiveFileExposureByFileType(ctx context.Context, fileType string) (int, error)
}
