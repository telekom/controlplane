// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	"context"
	"fmt"

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

	fileType, err := util.GetFileType(ctx, obj.Spec.FileType)
	if err != nil {
		return err
	}

	if fileType.Status.FileExposureRef == nil {
		resetServiceURLsInStatus(obj)
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, "No active FileExposure found for this FileType"))
		obj.SetCondition(condition.NewBlockedCondition("FileSubscription will be processed when a FileExposure is registered"))
		return nil
	}

	activeExposure := &filev1.FileExposure{}
	if err = c.Get(ctx, fileType.Status.FileExposureRef.K8s(), activeExposure); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			resetServiceURLsInStatus(obj)
			obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, "No active FileExposure found for this FileType"))
			obj.SetCondition(condition.NewBlockedCondition("FileSubscription will be processed when a FileExposure is registered"))
			return nil
		}
		return err
	}

	if !visibilityAllowsSubscription(activeExposure, obj) {
		resetServiceURLsInStatus(obj)
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "FileExposure and FileSubscription visibility combination is not allowed"))
		return ctrlerrors.BlockedErrorf("FileSubscription is blocked by FileExposure visibility")
	}

	obj.Status.FileTypeRef = types.ObjectRefFromObject(fileType)

	res, err := h.ensureApproval(ctx, obj, activeExposure)
	if err != nil {
		return err
	}
	switch res {
	case builder.ApprovalResultRequestDenied:
		logger.Info("ApprovalRequest was denied — not touching child resources")
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "ApprovalRequest has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("ApprovalRequest has been denied"))
		return nil
	case builder.ApprovalResultPending:
		logger.Info("Approval is pending — waiting for approval")
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonApprovalPending, "Waiting for approval decision"))
		obj.SetCondition(condition.NewBlockedCondition("Waiting for approval decision"))
		return nil
	case builder.ApprovalResultDenied:
		logger.Info("Approval was denied - deleting subscriber SFTP User")
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "Approval has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Approval has been denied"))
		err = h.deleteSubscriberUser(ctx, obj)
		if err != nil {
			return err
		}

		logger.Info("Subscriber SFTP User deleted due to approval denial")
		return nil
	case builder.ApprovalResultGranted:
		logger.Info("Approval is granted — continuing with provisioning")
		builder.ClearApprovalPendingReady(obj)
	default:
		return errors.Errorf("unknown approval-builder result %q", res)
	}

	err = h.syncSubscriberUser(ctx, obj, fileType, activeExposure)
	if err != nil {
		return err
	}

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, "One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady, "Waiting for child resources"))
		return nil
	}

	zoneServiceConfig, err := util.GetZoneServiceConfig(ctx, activeExposure.Spec.Zone)
	if err != nil {
		return err
	}

	obj.Status.ServiceURL = zoneServiceConfig.Spec.ServiceURL
	obj.Status.ServiceExternalURL = zoneServiceConfig.Spec.ServiceExternalURL
	obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned,
		"FileSubscription has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"FileSubscription has been provisioned"))
	return nil
}

func (h *FileSubscriptionHandler) Delete(ctx context.Context, obj *filev1.FileSubscription) error {
	return h.deleteSubscriberUser(ctx, obj)
}

func (h *FileSubscriptionHandler) ensureApproval(ctx context.Context, obj *filev1.FileSubscription, activeExposure *filev1.FileExposure) (builder.ApprovalResult, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	if obj.Spec.Requestor.Kind != "Application" {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonValidationFailed,
			"Only requestors of kind 'Application' are supported"))
		obj.SetCondition(condition.NewBlockedCondition(
			"EventSubscription with requestor kind " + obj.Spec.Requestor.Kind + " is not supported"))
		return builder.ApprovalResultNone, nil
	}
	requestorApp, err := util.GetApplication(ctx, obj.Spec.Requestor.ObjectRef)
	if err != nil {
		return builder.ApprovalResultNone, err
	}

	providerApp, err := util.GetApplication(ctx, activeExposure.Spec.Provider.ObjectRef)
	if err != nil {
		return builder.ApprovalResultNone, fmt.Errorf("unable to get application from FileExposure provider %q while handling FileSubscription %q: %w",
			activeExposure.Spec.Provider.Name, obj.Name, err)
	}

	requester := &approvalapi.Requester{
		TeamName:       requestorApp.Spec.Team,
		TeamEmail:      requestorApp.Spec.TeamEmail,
		ApplicationRef: &obj.Spec.Requestor,
		Reason: fmt.Sprintf("Team %s requested subscription to filetype %s from zone %s",
			requestorApp.Spec.Team, obj.Spec.FileType, obj.Spec.Zone.Name),
	}

	properties := map[string]any{
		"fileType":      obj.Spec.FileType,
		"resource_type": "filetype",
		"resource_name": obj.Spec.FileType,
	}

	err = requester.SetProperties(properties)
	if err != nil {
		return builder.ApprovalResultNone, fmt.Errorf("unable to set approvalRequest properties for FileSubscription %q in namespace %q: %w",
			obj.Name, obj.Namespace, err)
	}

	decider := &approvalapi.Decider{
		TeamName:       providerApp.Spec.Team,
		TeamEmail:      providerApp.Spec.TeamEmail,
		ApplicationRef: &activeExposure.Spec.Provider,
	}

	approvalBuilder := builder.NewApprovalBuilder(c, obj).
		WithAction("subscribe").
		WithHashValue(requester.Properties).
		WithRequester(requester).
		WithDecider(decider).
		WithLabels(util.DomainLabel()).
		WithStrategy(approvalapi.ApprovalStrategy(activeExposure.Spec.Approval.Strategy)).
		WithTrustedRequesters(activeExposure.Spec.Approval.TrustedTeams)

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
		util.GetPublicKeysFromSFTP(obj.Spec.SFTP),
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

	return exposure.Spec.Zone.Equals(subscription.Spec.Zone)
}

func subscriptionZoneName(subscription *filev1.FileSubscription) string {
	return subscription.Spec.Zone.Name
}

func resetServiceURLsInStatus(subscription *filev1.FileSubscription) {
	subscription.Status.ServiceURL = ""
	subscription.Status.ServiceExternalURL = ""
}
