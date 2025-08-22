// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func EnsureLabelsOrDie(ctx context.Context, obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	defer obj.SetLabels(labels)

	bCtx := ReceiveBCtxOrDie(ctx)

	labels[config.EnvironmentLabelKey] = bCtx.Environment
	labels[config.BuildLabelKey("team")] = bCtx.Team
	labels[config.BuildLabelKey("group")] = bCtx.Group
}

func ReceiveBCtxOrDie(ctx context.Context) *security.BusinessContext {
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		panic("security context not found")
	}

	return bCtx
}
