// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package user

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

var _ handler.Handler[*sftpv1.User] = &UserHandler{}

type UserHandler struct{}

func (h *UserHandler) CreateOrUpdate(ctx context.Context, obj *sftpv1.User) error {
	if obj.Spec.InstanceRef.IsEmpty() {
		return ctrlerrors.BlockedErrorf("Instance reference is required")
	}

	instance := &sftpv1.Instance{}
	if err := cclient.ClientFromContextOrDie(ctx).Get(ctx, obj.Spec.InstanceRef.K8s(), instance); err != nil {
		return fmt.Errorf("getting Instance %q for User %q: %w", obj.Spec.InstanceRef.String(), obj.Name, err)
	}

	processing := processingConditionFromInstance(instance, obj)
	processing.LastTransitionTime = metav1.Time{}
	obj.SetCondition(processing)
	obj.SetCondition(readyConditionFromProcessing(&processing))
	return nil
}

func (h *UserHandler) Delete(context.Context, *sftpv1.User) error {
	return nil
}

func processingConditionFromInstance(instance *sftpv1.Instance, user *sftpv1.User) metav1.Condition {
	if !isInstanceReady(instance) {
		return condition.NewProcessingCondition("WaitingForInstance", "Waiting for Instance to be ready")
	}

	userStatus := findInstanceUserStatus(instance, user)
	if userStatus == nil || userStatus.ProcessingCondition.ObservedGeneration != user.Generation {
		return condition.NewProcessingCondition("WaitingForInstance", "Waiting for Instance to process SSH public keys")
	}

	return userStatus.ProcessingCondition
}

func isInstanceReady(instance *sftpv1.Instance) bool {
	ready := meta.FindStatusCondition(instance.GetConditions(), condition.ConditionTypeReady)
	return ready != nil &&
		ready.Status == metav1.ConditionTrue &&
		ready.ObservedGeneration == instance.Generation
}

func findInstanceUserStatus(instance *sftpv1.Instance, user *sftpv1.User) *sftpv1.InstanceUserStatus {
	for i := range instance.Status.Users {
		userStatus := &instance.Status.Users[i]
		if userStatus.Namespace == user.Namespace && userStatus.Name == user.Name {
			return userStatus
		}
	}
	return nil
}

func readyConditionFromProcessing(processing *metav1.Condition) metav1.Condition {
	if processing.Status == metav1.ConditionFalse && processing.Reason == "Done" {
		return condition.NewReadyCondition(sftpv1.ConditionReadyReasonSSHPublicKeyProvided, "SSH public keys are ready")
	}

	return condition.NewNotReadyCondition(processing.Reason, processing.Message)
}
