// SPDX-FileCopyrightText: 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

type SpectreApplicationHandler struct{}

func (h *SpectreApplicationHandler) CreateOrUpdate(ctx context.Context, obj *spectrev1.SpectreApplication) error {
	return nil
}

func (h *SpectreApplicationHandler) Delete(ctx context.Context, obj *spectrev1.SpectreApplication) error {
	return nil
}
