// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package bouncer

import "context"

type Runnable func(ctx context.Context) error

type Bouncer interface {
	// Run starts the runnable and blocks until it is done.
	RunB(ctx context.Context, key string, runnable Runnable) error
}
