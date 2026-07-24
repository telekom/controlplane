// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
)

var _ handler.Handler[*filev1.FileSubscription] = &FileSubscriptionHandler{}

type FileSubscriptionHandler struct{}

func (h *FileSubscriptionHandler) CreateOrUpdate(ctx context.Context, obj *filev1.FileSubscription) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	fileType, err := util.GetFileType(ctx, types.ObjectRef{Namespace: obj.Namespace, Name: obj.Spec.FileType})
	if err != nil {
		return err
	}

	if fileType.Status.FileExposureRef == nil {
		obj.SetCondition(condition.NewNotReadyCondition("FileExposureNotFound", "No active FileExposure found for this FileType"))
		obj.SetCondition(condition.NewBlockedCondition("FileSubscription will be processed when a FileExposure is registered"))
		return nil
	}

	activeExposure := &filev1.FileExposure{}
	if err = c.Get(ctx, fileType.Status.FileExposureRef.K8s(), activeExposure); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			obj.SetCondition(condition.NewNotReadyCondition("FileExposureNotFound", "No active FileExposure found for this FileType"))
			obj.SetCondition(condition.NewBlockedCondition("FileSubscription will be processed when a FileExposure is registered"))
			return nil
		}
		return err
	}

	if !visibilityAllowsSubscription(activeExposure, obj) {
		obj.SetCondition(condition.NewNotReadyCondition("VisibilityConstraintViolation", "FileExposure and FileSubscription visibility combination is not allowed"))
		return ctrlerrors.BlockedErrorf("FileSubscription is blocked by FileExposure visibility")
	}

	obj.Status.FileTypeRef = types.ObjectRefFromObject(fileType)

	res, err := h.ensureApproval(ctx, obj, fileType, activeExposure)
	if err != nil {
		return err
	}
	switch res {
	case builder.ApprovalResultRequestDenied:
		logger.Info("ApprovalRequest was denied - deleting subscriber SFTP User")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalRequestDenied", "ApprovalRequest has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("ApprovalRequest has been denied"))
		return h.deleteSubscriberUser(ctx, obj)
	case builder.ApprovalResultPending:
		logger.Info("Approval is pending - waiting for approval")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Waiting for approval decision"))
		obj.SetCondition(condition.NewBlockedCondition("Waiting for approval decision"))
		return h.deleteSubscriberUser(ctx, obj)
	case builder.ApprovalResultDenied:
		logger.Info("Approval was denied - deleting subscriber SFTP User")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalDenied", "Approval has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Approval has been denied"))
		if cleanupErr := h.deleteSubscriberUser(ctx, obj); cleanupErr != nil {
			return fmt.Errorf("unable to cleanup SFTP User for FileSubscription %q in namespace %q: %w",
				obj.Name, obj.Namespace, cleanupErr)
		}
		return nil
	case builder.ApprovalResultGranted:
		logger.Info("Approval is granted - continuing with provisioning")
	default:
		return errors.Errorf("unknown approval-builder result %q", res)
	}

	err = h.syncSubscriberUser(ctx, obj, fileType, activeExposure)
	if err != nil {
		return err
	}

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady", "One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("FileSubscriptionProvisioned", "FileSubscription has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("FileSubscription has been provisioned"))
	return nil
}

func (h *FileSubscriptionHandler) Delete(ctx context.Context, obj *filev1.FileSubscription) error {
	return h.deleteSubscriberUser(ctx, obj)
}

func (h *FileSubscriptionHandler) ensureApproval(ctx context.Context, obj *filev1.FileSubscription, fileType *filev1.FileType, activeExposure *filev1.FileExposure) (builder.ApprovalResult, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	properties := approvalProperties(obj)

	requester := &approvalapi.Requester{
		TeamName:       teamNameFromNamespace(obj.Namespace),
		ApplicationRef: types.TypedObjectRefFromObject(obj, c.Scheme()),
		Reason: fmt.Sprintf("Team %s requested subscription to file type %s from zone %s",
			obj.Namespace, fileType.Name, subscriptionZoneName(obj)),
	}

	err := requester.SetProperties(properties)
	if err != nil {
		return builder.ApprovalResultNone, fmt.Errorf("unable to set approvalRequest properties for FileSubscription %q in namespace %q: %w",
			obj.Name, obj.Namespace, err)
	}

	decider := &approvalapi.Decider{
		TeamName:       teamNameFromNamespace(activeExposure.Namespace),
		ApplicationRef: types.TypedObjectRefFromObject(activeExposure, c.Scheme()),
	}

	approvalBuilder := builder.NewApprovalBuilder(c, obj)
	approvalBuilder.WithAction("subscribe")
	approvalBuilder.WithHashValue(requester.Properties)
	approvalBuilder.WithRequester(requester)
	approvalBuilder.WithDecider(decider)
	approvalBuilder.WithStrategy(approvalapi.ApprovalStrategy(activeExposure.Spec.Approval.Strategy))
	if len(activeExposure.Spec.Approval.TrustedTeams) > 0 {
		approvalBuilder.WithTrustedRequesters(activeExposure.Spec.Approval.TrustedTeams)
	}

	res, err := approvalBuilder.Build(ctx)
	if err != nil {
		return builder.ApprovalResultNone, err
	}
	obj.Status.ApprovalRequest = types.ObjectRefFromObject(approvalBuilder.GetApprovalRequest())
	obj.Status.Approval = types.ObjectRefFromObject(approvalBuilder.GetApproval())

	return res, nil
}

func (h *FileSubscriptionHandler) syncSubscriberUser(ctx context.Context, obj *filev1.FileSubscription, fileType *filev1.FileType, activeExposure *filev1.FileExposure) error {
	_, err := util.SyncSFTPUser(
		ctx,
		util.SFTPUserRefForFileSubscription(obj),
		obj,
		*types.ObjectRefFromObject(fileType),
		publicKeysFromSFTP(obj.Spec.SFTP),
		util.SFTPInstanceRefForFileExposure(activeExposure),
	)
	if err != nil {
		return fmt.Errorf("failed to sync subscriber SFTP User: %w", err)
	}
	return nil
}

func (h *FileSubscriptionHandler) deleteSubscriberUser(ctx context.Context, obj *filev1.FileSubscription) error {
	err := util.DeleteSFTPUser(ctx, util.SFTPUserRefForFileSubscription(obj))
	if err != nil {
		return fmt.Errorf("failed to delete subscriber SFTP User: %w", err)
	}
	return nil
}

func visibilityAllowsSubscription(exposure *filev1.FileExposure, subscription *filev1.FileSubscription) bool {
	if exposure.Spec.Visibility != filev1.VisibilityZone {
		return true
	}
	if exposure.Spec.Zone == nil || subscription.Spec.Zone == nil {
		return true
	}
	return exposure.Spec.Zone.Equals(subscription.Spec.Zone)
}

func approvalProperties(subscription *filev1.FileSubscription) map[string]any {
	return map[string]any{
		"fileType": subscription.Spec.FileType,
		"zone":     subscriptionZoneName(subscription),
	}
}

func subscriptionZoneName(subscription *filev1.FileSubscription) string {
	if subscription.Spec.Zone == nil {
		return ""
	}
	return subscription.Spec.Zone.Name
}

func teamNameFromNamespace(namespace string) string {
	parts := strings.Split(namespace, "--")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "--")
	}
	return namespace + "--" + namespace
}

func publicKeysFromSFTP(sftp *filev1.FileSFTP) []filev1.SSHPublicKeySpec {
	if sftp == nil {
		return nil
	}
	return sftp.PublicKeys
}
