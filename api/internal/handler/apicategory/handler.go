// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apicategory

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
)

var _ handler.Handler[*apiv1.ApiCategory] = (*ApiCategoryHandler)(nil)

type ApiCategoryHandler struct{}

func (a *ApiCategoryHandler) CreateOrUpdate(ctx context.Context, apiCategory *apiv1.ApiCategory) error {
	apiCategory.SetCondition(condition.NewReadyCondition("Provisioned", "API category resource has been provisioned successfully"))
	apiCategory.SetCondition(condition.NewDoneProcessingCondition("API category resource processing is complete"))
	return nil
}

func (a *ApiCategoryHandler) Delete(_ context.Context, _ *apiv1.ApiCategory) error {
	// nothing todo here yet
	return nil
}
