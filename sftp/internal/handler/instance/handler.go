// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
)

var _ handler.Handler[*sftpv1.Instance] = &InstanceHandler{}

type InstanceHandler struct {
	Client         client.Client
	ServiceFactory service.Factory
}

func (h *InstanceHandler) CreateOrUpdate(ctx context.Context, obj *sftpv1.Instance) error {
	if obj.Spec.ZoneServiceConfigRef.IsEmpty() {
		return ctrlerrors.BlockedErrorf("ZoneServiceConfig reference is required")
	}

	log := logr.FromContextOrDiscard(ctx)

	sftpService, err := h.serviceFor(ctx, obj.Spec.ZoneServiceConfigRef.K8s())
	if err != nil {
		return err
	}

	if obj.Generation != obj.Status.Generation {
		log.Info("Instance spec has changed, provisioning SFTP user in external service")
		obj.SetCondition(condition.NewProcessingCondition("Provisioning", "Instance is being provided"))

		err = h.createOrUpdateServiceUser(ctx, sftpService, obj)
		if err != nil {
			return err
		}
		obj.Status.Generation = obj.Generation
		err = h.Client.Status().Update(ctx, obj)
		if err != nil {
			return fmt.Errorf("updating status for Instance %q: %w", obj.Name, err)
		}
	}

	users, err := h.usersFor(ctx, obj)
	if err != nil {
		return err
	}

	sshPublicKeys := service.ClientPublicKeyMap{
		"items": collectUniquePublicKeysFromUsers(log, obj.Name, users),
	}

	err = sftpService.UpdatePublicKeysForSFTPUser(ctx, obj.Name, obj.Name, sshPublicKeys)
	if err != nil {
		obj.SetCondition(sftpv1.NewPublicKeysNotUpdatedInServiceCondition(err.Error()))
		return fmt.Errorf("updating public keys for SFTP user %q: %w", obj.Name, err)
	}

	err = h.updateUserStatus(ctx, users)
	if err != nil {
		return fmt.Errorf("updating status for Users of Instance %q: %w", obj.Name, err)
	}

	obj.SetCondition(sftpv1.NewPublicKeysUpdatedInServiceCondition())
	obj.SetCondition(condition.NewReadyCondition("InstanceProvided", "Instance has been provided"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Instance has been provided"))
	return nil
}

func (h *InstanceHandler) Delete(ctx context.Context, obj *sftpv1.Instance) error {
	if obj.Spec.ZoneServiceConfigRef.IsEmpty() {
		return nil
	}

	sftpService, err := h.serviceFor(ctx, obj.Spec.ZoneServiceConfigRef.K8s())
	if err != nil {
		return err
	}

	err = sftpService.DeleteSFTPUser(ctx, obj.Name)
	if err != nil {
		return fmt.Errorf("deleting SFTP user %q: %w", obj.Name, err)
	}

	return nil
}

func (h *InstanceHandler) usersFor(ctx context.Context, instance *sftpv1.Instance) ([]sftpv1.User, error) {
	list := &sftpv1.UserList{}
	if err := h.Client.List(ctx, list, client.InNamespace(instance.Namespace)); err != nil {
		return nil, fmt.Errorf("listing Users for Instance %q: %w", instance.Name, err)
	}

	users := make([]sftpv1.User, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Spec.InstanceRef.Equals(instance) {
			users = append(users, list.Items[i])
		}
	}
	return users, nil
}

func (h *InstanceHandler) serviceFor(ctx context.Context, zsc client.ObjectKey) (service.Service, error) {
	factory := h.ServiceFactory
	if factory == nil {
		factory = service.NewHTTPServiceFactory()
	}
	return factory.ServiceFor(ctx, zsc)
}

func (h *InstanceHandler) createOrUpdateServiceUser(ctx context.Context, sftpService service.Service, instance *sftpv1.Instance) error {
	events := []service.RoverSftpUserModelHorizonNotificationEvents{}

	sftpUser := service.RoverSftpUserModel{
		SftpUserName: instance.Name,
		// nil creates invalid user in an external service
		HorizonNotificationEvents: &events,
	}
	if instance.Spec.Description != "" {
		description := instance.Spec.Description
		sftpUser.Description = &description
	}

	if err := sftpService.CreateOrUpdateSFTPUser(ctx, sftpUser); err != nil {
		return fmt.Errorf("creating or updating SFTP user %q: %w", instance.Name, err)
	}

	return nil
}

func (h *InstanceHandler) updateUserStatus(ctx context.Context, users []sftpv1.User) error {
	var updateErrs error
	for i := range users {
		user := &users[i]
		if err := h.Client.Status().Update(ctx, user); err != nil {
			updateErrs = errors.Join(updateErrs, fmt.Errorf("updating status for User %q: %w", user.Name, err))
		}
	}
	return updateErrs
}

func collectUniquePublicKeysFromUsers(log logr.Logger, sftpUserName string, users []sftpv1.User) []service.RoverPublicKeyModel {
	if len(users) == 0 {
		return []service.RoverPublicKeyModel{}
	}

	fingerprints := map[string]*service.RoverPublicKeyModel{}
	keys := make([]service.RoverPublicKeyModel, 0, len(users))
	for i := range users {
		user := &users[i]
		hasInvalidSSHPublicKey := false
		for index, sshpublickey := range user.Spec.SSHPublicKeys {
			canonicalKey, err := sftpv1.CanonicalPublicKey(sshpublickey)
			if err != nil {
				hasInvalidSSHPublicKey = true
				log.Error(err, fmt.Sprintf("Failed to get canonical key (index: %d)", index), "user", user.Namespace+"/"+user.Name)
				continue
			}

			fingerprint, err := sftpv1.FingerprintForKey(canonicalKey)
			if err != nil {
				hasInvalidSSHPublicKey = true
				log.Error(err, fmt.Sprintf("Failed to process public key (index: %d)", index), "user", user.Namespace+"/"+user.Name)
				continue
			}

			targetKey, exists := fingerprints[fingerprint]
			desc := user.Namespace + "/" + user.Name
			if exists {
				// use first user for key description, if exist multiple clients
				if *targetKey.Description < desc {
					targetKey.Description = &desc
				}
			} else {
				keys = append(keys, service.RoverPublicKeyModel{
					PublicKey:    canonicalKey,
					SftpUserName: sftpUserName,
					Description:  &desc,
				})
				fingerprints[fingerprint] = &keys[len(keys)-1]
			}
		}

		setUserConditions(user, hasInvalidSSHPublicKey)
	}

	return keys
}

func setUserConditions(user *sftpv1.User, hasInvalidSSHPublicKey bool) {
	done := condition.NewDoneProcessingCondition("SSH Public keys were processed")
	ready := condition.NewReadyCondition(sftpv1.ConditionReadyReasonSSHPublicKeyProvided, "SSH Public keys are ready")
	if hasInvalidSSHPublicKey {
		done = condition.NewDoneProcessingCondition("Failed to process public key")
		ready = condition.NewNotReadyCondition(
			sftpv1.ConditionReadyReasonSSHPublicKeyProvided,
			"Failed to process public key")
	}
	user.SetCondition(done)
	user.SetCondition(ready)
}
