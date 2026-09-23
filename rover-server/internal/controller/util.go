// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func EnsureLabelsOrDie(ctx context.Context, obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	defer obj.SetLabels(labels)

	bCtx, ok := security.FromContext(ctx)
	if !ok {
		panic("security context not found")
	}

	labels[config.EnvironmentLabelKey] = labelutil.NormalizeLabelValue(bCtx.Environment)
	labels[config.BuildLabelKey("team")] = labelutil.NormalizeLabelValue(bCtx.Team)
	labels[config.BuildLabelKey("group")] = labelutil.NormalizeLabelValue(bCtx.Group)
}
