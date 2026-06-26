// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure

import (
	"context"
	"fmt"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ handler.Handler[*filev1.FileExposure] = &FileExposureHandler{}

type FileExposureHandler struct{}

func (h *FileExposureHandler) CreateOrUpdate(ctx context.Context, obj *filev1.FileExposure) error {
	c := cclient.ClientFromContextOrDie(ctx)

	fileType, err := util.GetFileType(ctx, types.ObjectRef{Namespace: obj.Namespace, Name: obj.Spec.FileType})
	if err != nil {
		return err
	}

	if obj.Spec.Zone == nil {
		return ctrlerrors.BlockedErrorf("ZoneServiceConfig reference is required")
	}

	if _, err = util.GetZoneServiceConfig(ctx, *obj.Spec.Zone); err != nil {
		return err
	}

	activeExposure, found, err := util.FindActiveFileExposure(ctx, fileType)
	if err != nil {
		return err
	}

	if found && activeExposure.UID != obj.UID {
		obj.Status.FileTypeRef = types.ObjectRefFromObject(fileType)
		obj.SetCondition(condition.NewNotReadyCondition("FileExposureAlreadyExists", "Another FileExposure already provides this FileType"))
		obj.SetCondition(condition.NewBlockedCondition("FileExposure will be processed when the active FileExposure is deleted"))
		return nil
	}

	if err = h.createOrUpdateInstance(ctx, obj, fileType); err != nil {
		return err
	}

	obj.Status.FileTypeRef = types.ObjectRefFromObject(fileType)

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady", "One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("FileExposureProvisioned", "FileExposure has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("FileExposure has been provisioned"))
	return nil
}

func (h *FileExposureHandler) Delete(ctx context.Context, obj *filev1.FileExposure) error {
	return nil
}

func (h *FileExposureHandler) createOrUpdateInstance(ctx context.Context, obj *filev1.FileExposure, fileType *filev1.FileType) error {
	c := cclient.ClientFromContextOrDie(ctx)
	instanceRef := util.SFTPInstanceRefForFileExposure(obj)
	instance := &sftpv1.Instance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceRef.Name,
			Namespace: instanceRef.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, instance, c.Scheme()); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		instance.Labels = util.ChildLabels(*types.ObjectRefFromObject(fileType))
		instance.Spec.Description = fileType.Spec.Description
		instance.Spec.ZoneServiceConfigRef = *obj.Spec.Zone
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, instance, mutator); err != nil {
		return fmt.Errorf("failed to create or update SFTP Instance %q: %w", instance.Name, err)
	}
	return nil
}
