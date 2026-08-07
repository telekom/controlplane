// Copyright 2026 Deutsche Telekom IT GmbH
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
	"github.com/telekom/controlplane/file/internal/index"
)

const (
	identityClientNamePrefix = "sftp-api"
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

func GetChildResourceRef(obj *filev1.ZoneServiceConfig) types.ObjectRef {
	return types.ObjectRef{
		Name:      identityClientNamePrefix + "--" + obj.Name,
		Namespace: obj.Namespace,
	}
}

func GetZoneServiceConfig(ctx context.Context, ref *types.ObjectRef) (*filev1.ZoneServiceConfig, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	list := &filev1.ZoneServiceConfigList{}
	err := c.List(ctx, list, client.InNamespace(GetZoneNamespace(ref)), client.MatchingFields{index.FieldSpecZoneOnZoneServiceConfig: ref.String()})
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, ctrlerrors.BlockedErrorf("ZoneServiceConfig %q not found", ref.String())
		}
		return nil, fmt.Errorf("failed to get ZoneServiceConfig %q: %w", ref.String(), err)
	}

	if len(list.Items) != 1 {
		return nil, ctrlerrors.BlockedErrorf("expected exactly one ZoneServiceConfig for zone %q, but found %d", ref.String(), len(list.Items))
	}

	zoneServiceConfig := &list.Items[0]
	return zoneServiceConfig, nil
}

func FindFileExposuresForFileType(ctx context.Context, fileType *types.ObjectRef) ([]filev1.FileExposure, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	list := &filev1.FileExposureList{}
	if err := c.List(ctx, list,
		client.InNamespace(fileType.Namespace),
		client.MatchingFields{index.FieldSpecFileTypeOnExposure: fileType.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list FileExposures for FileType %q: %w", fileType.Name, err)
	}

	exposures := make([]filev1.FileExposure, len(list.Items))
	copy(exposures, list.Items)

	slices.SortFunc(exposures, func(i, j filev1.FileExposure) int {
		cmp := i.CreationTimestamp.Compare(j.CreationTimestamp.Time)
		if cmp == 0 {
			return strings.Compare(i.Name, j.Name)
		}
		return cmp
	})

	return exposures, nil
}

func FindActiveFileExposure(ctx context.Context, fileType *types.ObjectRef) (*filev1.FileExposure, bool, error) {
	exposures, err := FindFileExposuresForFileType(ctx, fileType)
	if err != nil {
		return nil, false, err
	}
	if len(exposures) == 0 {
		return nil, false, nil
	}
	return &exposures[0], true, nil
}

func GetPublicKeysFromSFTP(sftp *filev1.FileSFTP) []filev1.SSHPublicKeySpec {
	if sftp == nil {
		return nil
	}
	return sftp.PublicKeys
}

func GetZoneNamespace(ref *types.ObjectRef) string {
	return ref.Namespace + "--" + ref.Name
}
