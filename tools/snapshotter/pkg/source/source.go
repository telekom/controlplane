// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

type Source interface {
	TakeSnapshot(ctx context.Context, resourceType, resourceId string) (snap snapshot.Snapshot, err error)
	TakeGlobalSnapshot(ctx context.Context, resourceType string, limit int) (snap map[string]snapshot.Snapshot, err error)
}
