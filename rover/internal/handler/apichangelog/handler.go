// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.ApiChangelog] = (*ApiChangelogHandler)(nil)

type ApiChangelogHandler struct{}

func (h *ApiChangelogHandler) CreateOrUpdate(ctx context.Context, changelog *roverv1.ApiChangelog) error {
	changelog.SetCondition(condition.NewDoneProcessingCondition("ApiChangelog created"))
	changelog.SetCondition(condition.NewReadyCondition("Ready", "ApiChangelog is ready"))
	return nil
}

func (h *ApiChangelogHandler) Delete(ctx context.Context, changelog *roverv1.ApiChangelog) error {
	return nil
}
