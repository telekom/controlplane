// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/types"
)

type NopHandler[T types.Object] struct{}

func NewNopHandler[T types.Object]() *NopHandler[T] {
	return &NopHandler[T]{}
}

func (h *NopHandler[T]) CreateOrUpdate(ctx context.Context, object T) error {
	return nil
}

func (h *NopHandler[T]) Delete(ctx context.Context, obj T) error {
	return nil
}
