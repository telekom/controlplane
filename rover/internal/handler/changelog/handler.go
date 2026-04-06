// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changelog

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.Changelog] = (*ChangelogHandler)(nil)

type ChangelogHandler struct{}

func (h *ChangelogHandler) CreateOrUpdate(ctx context.Context, changelog *roverv1.Changelog) error {
	changelog.SetCondition(condition.NewDoneProcessingCondition("Changelog created"))
	changelog.SetCondition(condition.NewReadyCondition("Ready", "Changelog is ready"))
	return nil
}

func (h *ChangelogHandler) Delete(ctx context.Context, changelog *roverv1.Changelog) error {
	return nil
}
