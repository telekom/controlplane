// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
)

var _ handler.Handler[*filev1.FileType] = &FileTypeHandler{}

type FileTypeHandler struct{}

func (h *FileTypeHandler) CreateOrUpdate(ctx context.Context, obj *filev1.FileType) error {
	activeExposure, found, err := util.FindActiveFileExposure(ctx, obj.Name)
	if err != nil {
		return err
	}
	if !found {
		obj.Status.Active = false
		obj.Status.FileExposureRef = nil
		obj.Status.SFTPInstance = nil
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, "No FileExposure found for this FileType"))
		obj.SetCondition(condition.NewBlockedCondition("FileType will be processed when a FileExposure is registered"))
		return nil
	}

	_, err = util.GetZoneServiceConfig(ctx, activeExposure.Spec.Zone)
	if err != nil {
		return err
	}

	obj.Status.FileExposureRef = types.ObjectRefFromObject(activeExposure)
	obj.Status.SFTPInstance = &types.ObjectRef{
		Name:      obj.Name,
		Namespace: activeExposure.Namespace,
	}

	obj.Status.Active = true
	obj.SetCondition(condition.NewReadyCondition("FileTypeProvisioned", "FileType has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("FileType has been provisioned"))
	return nil
}

func (h *FileTypeHandler) Delete(ctx context.Context, obj *filev1.FileType) error {
	if obj.Status.FileExposureRef != nil {
		return ctrlerrors.BlockedErrorf("cannot delete FileType while it is still associated with a FileExposure")
	}

	return nil
}
