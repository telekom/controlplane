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
	// see predefined errors here: https://telekom.github.io/controlplane/docs/developer-journey/creating-an-operator#error-handling
	return nil
}

func (ls *ListenerHandler) Delete(ctx context.Context, listener *spectrev1.Listener) error {
	// todo: add actual logic, if needed
	// see predefined errors here: https://telekom.github.io/controlplane/docs/developer-journey/creating-an-operator#error-handling
	return nil
}
