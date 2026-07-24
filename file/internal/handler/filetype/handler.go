// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

var _ handler.Handler[*filev1.FileType] = &FileTypeHandler{}

type FileTypeHandler struct{}

func (h *FileTypeHandler) CreateOrUpdate(ctx context.Context, obj *filev1.FileType) error {
	c := cclient.ClientFromContextOrDie(ctx)

	activeExposure, found, err := util.FindActiveFileExposure(ctx, obj)
	if err != nil {
		return err
	}
	if !found {
		err = h.deleteProjectedUser(ctx, obj)
		if err != nil {
			return err
		}
		obj.Status.FileExposureRef = nil
		obj.SetCondition(condition.NewNotReadyCondition("FileExposureNotFound", "No FileExposure found for this FileType"))
		obj.SetCondition(condition.NewBlockedCondition("FileType will be processed when a FileExposure is registered"))
		return nil
	}

	if activeExposure.Spec.Zone == nil {
		return ctrlerrors.BlockedErrorf("ZoneServiceConfig reference is required")
	}

	_, err = util.GetZoneServiceConfig(ctx, *activeExposure.Spec.Zone)
	if err != nil {
		return err
	}

	userRef := client.ObjectKeyFromObject(obj)
	user := &sftpv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:            userRef.Name,
			Namespace:       userRef.Namespace,
			OwnerReferences: obj.GetOwnerReferences(),
		},
	}

	mutator := func() error {
		if locErr := controllerutil.SetControllerReference(obj, user, c.Scheme()); locErr != nil {
			return fmt.Errorf("failed to set controller reference: %w", locErr)
		}

		user.Labels = util.ChildLabels(*types.ObjectRefFromObject(obj))
		user.Spec.InstanceRef = util.SFTPInstanceRefForFileExposure(activeExposure)
		keys, locErr := util.CanonicalSSHPublicKeys(publicKeysFromSFTP(activeExposure.Spec.SFTP))
		if locErr != nil {
			return locErr
		}
		user.Spec.SSHPublicKeys = keys
		return nil
	}

	if _, err = c.CreateOrUpdate(ctx, user, mutator); err != nil {
		return fmt.Errorf("failed to create or update SFTP User %q: %w", user.Name, err)
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

	obj.SetCondition(condition.NewReadyCondition("FileTypeProvisioned", "FileType has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("FileType has been provisioned"))
	return nil
}

func (h *FileTypeHandler) Delete(ctx context.Context, obj *filev1.FileType) error {
	return h.deleteProjectedUser(ctx, obj)
}

func (h *FileTypeHandler) deleteProjectedUser(ctx context.Context, obj *filev1.FileType) error {
	c := cclient.ClientFromContextOrDie(ctx)

	userKey := client.ObjectKeyFromObject(obj)

	user := &sftpv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userKey.Name,
			Namespace: userKey.Namespace,
		},
	}

	if err := c.Delete(ctx, user); err != nil && !apierrors.IsNotFound(errors.Cause(err)) {
		return fmt.Errorf("failed to delete SFTP User %q: %w", user.Name, err)
	}

	return nil
}

func publicKeysFromSFTP(sftp *filev1.FileSFTP) []filev1.SSHPublicKeySpec {
	if sftp == nil {
		return nil
	}
	return sftp.PublicKeys
}
