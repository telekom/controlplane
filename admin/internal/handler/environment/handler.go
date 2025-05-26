// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
)

var _ handler.Handler[*adminv1.Environment] = &EnvironmentHandler{}

type EnvironmentHandler struct{}

func (h *EnvironmentHandler) CreateOrUpdate(ctx context.Context, obj *adminv1.Environment) error {
	obj.SetCondition(condition.NewReadyCondition("EnvironmentProvided", "Environment has been provided"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Environment has been provided"))

	return nil
}

func (h *EnvironmentHandler) Delete(ctx context.Context, obj *adminv1.Environment) error {
	return nil
}
