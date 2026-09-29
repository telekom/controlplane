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

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/index"
)

const (
	identityClientNamePrefix = "sftp-api"
)

func GetFileType(ctx context.Context, name string) (*filev1.FileType, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	list := &filev1.FileTypeList{}
	if err := c.List(ctx, list, client.MatchingFields{index.FileTypeName: name}); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, ctrlerrors.BlockedErrorf("FileType %q not found", name)
		}
		return nil, fmt.Errorf("failed to get FileType %q: %w", name, err)
	}
	if len(list.Items) != 1 {
		return nil, ctrlerrors.BlockedErrorf("expected exactly one FileType with name %q, but found %d", name, len(list.Items))
	}

	return &list.Items[0], nil
}

func GetChildResourceRef(obj *filev1.ZoneServiceConfig) types.ObjectRef {
	return types.ObjectRef{
		Name:      identityClientNamePrefix + "-" + obj.Name,
		Namespace: obj.Namespace,
	}
}

func GetZoneServiceConfig(ctx context.Context, zoneRef *types.ObjectRef) (*filev1.ZoneServiceConfig, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zoneNamespace, err := FetchZoneNamespace(ctx, zoneRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone namespace for %q: %w", zoneRef.String(), err)
	}

	list := &filev1.ZoneServiceConfigList{}
	err = c.List(ctx, list, client.InNamespace(zoneNamespace), client.MatchingFields{index.FieldSpecZoneOnZoneServiceConfig: zoneRef.String()})
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, ctrlerrors.BlockedErrorf("ZoneServiceConfig %q not found", zoneRef.String())
		}
		return nil, fmt.Errorf("failed to get ZoneServiceConfig %q: %w", zoneRef.String(), err)
	}

	if len(list.Items) != 1 {
		return nil, ctrlerrors.BlockedErrorf("expected exactly one ZoneServiceConfig for zone %q, but found %d", zoneRef.String(), len(list.Items))
	}

	zoneServiceConfig := &list.Items[0]
	return zoneServiceConfig, nil
}

func FindFileExposuresForFileType(ctx context.Context, fileTypeName string) ([]filev1.FileExposure, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	list := &filev1.FileExposureList{}
	if err := c.List(ctx, list,
		client.MatchingFields{index.FieldSpecFileTypeOnExposure: fileTypeName},
	); err != nil {
		return nil, fmt.Errorf("failed to list FileExposures for FileType %q: %w", fileTypeName, err)
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

func FindActiveFileExposure(ctx context.Context, fileTypeName string) (*filev1.FileExposure, bool, error) {
	exposures, err := FindFileExposuresForFileType(ctx, fileTypeName)
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

func FetchZoneNamespace(ctx context.Context, ref *types.ObjectRef) (string, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	zone := &adminv1.Zone{}
	err := c.Get(ctx, ref.K8s(), zone)
	if err != nil {
		return "", err
	}

	return zone.Status.Namespace, nil
}

// GetApplication retrieves an Application object by ObjectRef and ensures it is ready.
func GetApplication(ctx context.Context, ref types.ObjectRef) (*applicationv1.Application, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	application := &applicationv1.Application{}
	err := c.Get(ctx, ref.K8s(), application)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("application %q not found", ref.String())
		}
		return nil, errors.Wrapf(err, "failed to get application %q", ref.String())
	}
	if err := condition.EnsureReady(application); err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q is not ready", ref.String())
	}

	return application, nil
}
