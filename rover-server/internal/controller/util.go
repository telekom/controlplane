// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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

// resolveFileId returns the file ID already stored on the resource, or a new UUIDv7 if none exists.
func resolveFileId(existingFileId string) (string, error) {
	if existingFileId != "" {
		return existingFileId, nil
	}
	newId, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generating file id: %w", err)
	}
	return newId.String(), nil
}
