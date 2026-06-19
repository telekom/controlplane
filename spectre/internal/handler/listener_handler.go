// SPDX-FileCopyrightText: 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

type ListenerHandler struct{}

func (ls *ListenerHandler) CreateOrUpdate(ctx context.Context, listener *spectrev1.Listener) error {
	// todo: add actual logic
	return nil
}

func (ls *ListenerHandler) Delete(ctx context.Context, listener *spectrev1.Listener) error {
	// todo: add actual logic, if needed
	return nil
}
