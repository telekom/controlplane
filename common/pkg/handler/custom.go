// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/types"
)

type (
	CreateOrUpdateFunc[T types.Object] func(ctx context.Context, object T) error
	DeleteFunc[T types.Object]         func(ctx context.Context, obj T) error
)

type CustomHandler[T types.Object] struct {
	createOrUpdate CreateOrUpdateFunc[T]
	deleteFunc     DeleteFunc[T]
}

func NewCustomHandler[T types.Object](createOrUpdate CreateOrUpdateFunc[T], deleteFunc DeleteFunc[T]) *CustomHandler[T] {
	return &CustomHandler[T]{
		createOrUpdate: createOrUpdate,
		deleteFunc:     deleteFunc,
	}
}

func (h *CustomHandler[T]) CreateOrUpdate(ctx context.Context, object T) error {
	return h.createOrUpdate(ctx, object)
}

func (h *CustomHandler[T]) Delete(ctx context.Context, obj T) error {
	return h.deleteFunc(ctx, obj)
}
