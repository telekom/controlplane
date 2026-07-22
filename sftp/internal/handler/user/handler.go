// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package user

import (
	"context"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*sftpv1.User] = &UserHandler{}

type UserHandler struct {
	serviceFactory service.Factory
}

func New(serviceFactory service.Factory) (*UserHandler, error) {
	if serviceFactory == nil {
		return nil, fmt.Errorf("service factory is required")
	}

	return &UserHandler{serviceFactory: serviceFactory}, nil
}

func (h *UserHandler) CreateOrUpdate(ctx context.Context, obj *sftpv1.User) error {
	if obj.Spec.InstanceRef.IsEmpty() {
		return ctrlerrors.BlockedErrorf("Instance reference is required")
	}

	instance := &sftpv1.Instance{}
	if err := cclient.ClientFromContextOrDie(ctx).Get(ctx, obj.Spec.InstanceRef.K8s(), instance); err != nil {
		return fmt.Errorf("getting Instance %q for User %q: %w", obj.Spec.InstanceRef.String(), obj.Name, err)
	}

	if instance.Spec.SFTPServiceConfigRef.IsEmpty() {
		return ctrlerrors.BlockedErrorf("SFTPServiceConfig reference is required")
	}

	if !condition.IsReady(instance) {
		obj.SetCondition(condition.NewNotReadyCondition("WaitingForInstance", "Waiting for Instance to be ready"))
		return nil
	}

	log := logf.FromContext(ctx)
	conditionReady := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if conditionReady != nil && conditionReady.ObservedGeneration == obj.Generation && conditionReady.Status == v1.ConditionTrue {
		log.Info("User spec didn't change, skipping updating of SSH Public keys in external service")
		return nil
	}

	log.Info("User spec has changed, updating SSH Public keys in external service")

	obj.SetCondition(condition.NewProcessingCondition("UpdatingPublicKeys", "Updating SSH public keys in external service"))

	sftpService, err := h.serviceFactory.ServiceFor(ctx, instance.Spec.SFTPServiceConfigRef.K8s())
	if err != nil {
		return err
	}

	sshPublicKeys := getRoverPublicKeys(obj.Spec.SSHPublicKeys, instance.Name, userClientID(obj))

	err = sftpService.UpdatePublicKeysForSFTPUser(ctx, instance.Name, userClientID(obj), sshPublicKeys)
	if err != nil {
		return fmt.Errorf("updating public keys for User %q on SFTP user %q: %w", obj.Name, instance.Name, err)
	}

	processing := condition.NewDoneProcessingCondition("SSH public keys were processed")
	obj.SetCondition(processing)
	obj.SetCondition(condition.NewReadyCondition("SSHPublicKeysUpdated", "SSH public keys have been updated in service"))
	return nil
}

func (h *UserHandler) Delete(ctx context.Context, obj *sftpv1.User) error {
	if obj.Spec.InstanceRef.IsEmpty() {
		return nil
	}

	instance := &sftpv1.Instance{}
	err := cclient.ClientFromContextOrDie(ctx).Get(ctx, obj.Spec.InstanceRef.K8s(), instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting Instance %q for User %q: %w", obj.Spec.InstanceRef.String(), obj.Name, err)
	}

	if instance.Spec.SFTPServiceConfigRef.IsEmpty() {
		return nil
	}

	sftpService, err := h.serviceFactory.ServiceFor(ctx, instance.Spec.SFTPServiceConfigRef.K8s())
	if err != nil {
		return err
	}

	clientID := userClientID(obj)

	err = sftpService.UpdatePublicKeysForSFTPUser(ctx, instance.Name, clientID, getRoverPublicKeys(nil, instance.Name, clientID))
	if err != nil {
		return fmt.Errorf("removing public keys for User %q on SFTP user %q: %w", obj.Name, instance.Name, err)
	}

	return nil
}

func getRoverPublicKeys(keys []string, instanceName, clientID string) service.ClientPublicKeyMap {
	const keyItems string = "items"
	if len(keys) == 0 {
		return service.ClientPublicKeyMap{keyItems: []service.RoverPublicKeyModel{}}
	}

	publicKeys := make([]service.RoverPublicKeyModel, 0, len(keys))
	for index, sshPublicKey := range keys {
		description := clientID + "/" + strconv.FormatInt(int64(index), 10)
		publicKeys = append(publicKeys, service.RoverPublicKeyModel{
			PublicKey:    sshPublicKey,
			SftpUserName: instanceName,
			Description:  &description,
		})
	}

	return service.ClientPublicKeyMap{keyItems: publicKeys}
}

func userClientID(user *sftpv1.User) string {
	return user.Namespace + "/" + user.Name
}
