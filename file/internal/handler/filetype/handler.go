// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import (
	"context"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
)

var _ handler.Handler[*filev1.FileType] = &FileTypeHandler{}

type FileTypeHandler struct{}

func (h *FileTypeHandler) CreateOrUpdate(ctx context.Context, obj *filev1.FileType) error {
	c := cclient.ClientFromContextOrDie(ctx)

	activeExposure, found, err := util.FindActiveFileExposure(ctx, types.ObjectRefFromObject(obj))
	if err != nil {
		return err
	}
	if !found {
		obj.Status.Active = false
		obj.Status.FileExposureRef = nil
		obj.SetCondition(condition.NewNotReadyCondition("FileExposureNotFound", "No FileExposure found for this FileType"))
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
		Namespace: obj.Namespace,
	}

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady", "One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	obj.Status.Active = true
	obj.SetCondition(condition.NewReadyCondition("FileTypeProvisioned", "FileType has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("FileType has been provisioned"))
	return nil
}

func (h *FileTypeHandler) Delete(ctx context.Context, obj *filev1.FileType) error {
	return nil
}
