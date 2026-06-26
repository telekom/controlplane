// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
)

func GetFileType(ctx context.Context, ref types.ObjectRef) (*filev1.FileType, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	fileType := &filev1.FileType{}
	if err := c.Get(ctx, ref.K8s(), fileType); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, ctrlerrors.BlockedErrorf("FileType %q not found", ref.String())
		}
		return nil, fmt.Errorf("failed to get FileType %q: %w", ref.String(), err)
	}
	return fileType, nil
}

func GetZoneServiceConfig(ctx context.Context, ref types.ObjectRef) (*filev1.ZoneServiceConfig, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	zoneServiceConfig := &filev1.ZoneServiceConfig{}
	if err := c.Get(ctx, ref.K8s(), zoneServiceConfig); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, ctrlerrors.BlockedErrorf("ZoneServiceConfig %q not found", ref.String())
		}
		return nil, fmt.Errorf("failed to get ZoneServiceConfig %q: %w", ref.String(), err)
	}
	return zoneServiceConfig, nil
}

func FindFileExposuresForFileType(ctx context.Context, fileType *filev1.FileType) ([]filev1.FileExposure, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	list := &filev1.FileExposureList{}
	if err := c.List(ctx, list, &client.ListOptions{
		Namespace: fileType.Namespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list FileExposures for FileType %q: %w", fileType.Name, err)
	}

	exposures := make([]filev1.FileExposure, 0, 2)
	for i := range list.Items {
		if list.Items[i].Spec.FileType == fileType.Name {
			exposures = append(exposures, list.Items[i])
		}
	}

	slices.SortFunc(exposures, func(i, j filev1.FileExposure) int {
		cmp := i.CreationTimestamp.Compare(j.CreationTimestamp.Time)
		if cmp == 0 {
			return strings.Compare(i.Name, j.Name)
		}
		return cmp
	})

	return exposures, nil
}

func FindActiveFileExposure(ctx context.Context, fileType *filev1.FileType) (*filev1.FileExposure, bool, error) {
	exposures, err := FindFileExposuresForFileType(ctx, fileType)
	if err != nil {
		return nil, false, err
	}
	if len(exposures) == 0 {
		return nil, false, nil
	}
	return &exposures[0], true, nil
}
