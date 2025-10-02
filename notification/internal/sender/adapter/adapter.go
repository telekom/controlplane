// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
)

type NotificationAdapter[C any] interface {
	Send(ctx context.Context, config *C, title string, body string) error
}
