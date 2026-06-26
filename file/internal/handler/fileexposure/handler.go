// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

var _ handler.Handler[*filev1.FileExposure] = &FileExposureHandler{}

type FileExposureHandler struct{}

func (h *FileExposureHandler) CreateOrUpdate(ctx context.Context, obj *filev1.FileExposure) error {
	c := cclient.ClientFromContextOrDie(ctx)

	if obj.Spec.Zone == nil {
		return ctrlerrors.BlockedErrorf("ZoneServiceConfig reference is required")
	}

	fileType, err := util.GetFileType(ctx, types.ObjectRef{Namespace: obj.Namespace, Name: obj.Spec.FileType})
	if err != nil {
		return err
	}

	zoneServiceConfig, err := util.GetZoneServiceConfig(ctx, *obj.Spec.Zone)
	if err != nil {
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

	err = h.createOrUpdateInstance(ctx, obj, fileType, zoneServiceConfig)
	if err != nil {
		return err
	}

	err = h.createOrUpdateProviderUser(ctx, obj, fileType)
	if err != nil {
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
	if err := util.DeleteSFTPUser(ctx, util.SFTPUserRefForFileExposure(obj)); err != nil {
		return fmt.Errorf("failed to delete provider SFTP User: %w", err)
	}
	return nil
}

func (h *FileExposureHandler) createOrUpdateProviderUser(ctx context.Context, obj *filev1.FileExposure, fileType *filev1.FileType) error {
	_, err := util.SyncSFTPUser(
		ctx,
		util.SFTPUserRefForFileExposure(obj),
		obj,
		*types.ObjectRefFromObject(fileType),
		publicKeysFromSFTP(obj.Spec.SFTP),
		util.SFTPInstanceRefForFileExposure(obj),
	)
	if err != nil {
		return fmt.Errorf("failed to sync provider SFTP User: %w", err)
	}
	return nil
}

func publicKeysFromSFTP(sftp *filev1.FileSFTP) []filev1.SSHPublicKeySpec {
	if sftp == nil {
		return nil
	}
	return sftp.PublicKeys
}

func (h *FileExposureHandler) createOrUpdateInstance(ctx context.Context, obj *filev1.FileExposure, fileType *filev1.FileType, zoneServiceConfig *filev1.ZoneServiceConfig) error {
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
		instance.Spec.SFTPServiceConfigRef = util.SFTPServiceConfigRefForZoneServiceConfig(zoneServiceConfig)
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, instance, mutator); err != nil {
		return fmt.Errorf("failed to create or update SFTP Instance %q: %w", instance.Name, err)
	}
	return nil
}
