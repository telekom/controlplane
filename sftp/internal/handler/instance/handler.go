// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
)

var _ handler.Handler[*sftpv1.Instance] = &InstanceHandler{}

type InstanceHandler struct {
	Client         client.Client
	ServiceFactory service.Factory
}

func (h *InstanceHandler) CreateOrUpdate(ctx context.Context, obj *sftpv1.Instance) error {
	if obj.Spec.SFTPServiceConfigRef.IsEmpty() {
		return ctrlerrors.BlockedErrorf("SFTPServiceConfig reference is required")
	}

	log := logr.FromContextOrDiscard(ctx)

	sftpService, err := h.serviceFor(ctx, obj.Spec.SFTPServiceConfigRef.K8s())
	if err != nil {
		return err
	}

	conditionReady := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if conditionReady == nil || conditionReady.ObservedGeneration != obj.Generation || conditionReady.Status != v1.ConditionTrue {
		log.Info("Instance spec has changed, provisioning SFTP user in external service")
		obj.SetCondition(condition.NewProcessingCondition("Provisioning", "Instance is being provided"))

		err = h.createOrUpdateServiceUser(ctx, sftpService, obj)
		if err != nil {
			return err
		}
	}

	users, err := h.usersFor(ctx, obj)
	if err != nil {
		return err
	}

	publicKeys, userStatuses := collectUniquePublicKeysFromUsers(log, obj.Name, users)
	sshPublicKeys := service.ClientPublicKeyMap{
		"items": publicKeys,
	}

	err = sftpService.UpdatePublicKeysForSFTPUser(ctx, obj.Name, obj.Name, sshPublicKeys)
	if err != nil {
		obj.SetCondition(sftpv1.NewPublicKeysNotUpdatedInServiceCondition(err.Error()))
		return fmt.Errorf("updating public keys for SFTP user %q: %w", obj.Name, err)
	}

	preserveUserStatusesForSameGeneration(obj.Status.Users, userStatuses)
	obj.Status.Users = userStatuses
	obj.SetCondition(sftpv1.NewPublicKeysUpdatedInServiceCondition())
	obj.SetCondition(condition.NewReadyCondition("InstanceProvided", "Instance has been provided"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Instance has been provided"))
	return nil
}

func (h *InstanceHandler) Delete(ctx context.Context, obj *sftpv1.Instance) error {
	if obj.Spec.SFTPServiceConfigRef.IsEmpty() {
		return nil
	}

	sftpService, err := h.serviceFor(ctx, obj.Spec.SFTPServiceConfigRef.K8s())
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
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	list := &sftpv1.UserList{}
	err := scopedClient.List(ctx, list, client.MatchingFields{sftpv1.IndexFieldSpecInstanceRef: commontypes.ObjectRefFromObject(instance).String()})
	if err != nil {
		return nil, fmt.Errorf("listing Users for Instance %q: %w", instance.Name, err)
	}

	return list.Items, nil
}

func (h *InstanceHandler) serviceFor(ctx context.Context, sftpServiceConfig client.ObjectKey) (service.Service, error) {
	factory := h.ServiceFactory
	if factory == nil {
		factory = service.NewHTTPServiceFactory()
	}
	return factory.ServiceFor(ctx, sftpServiceConfig)
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

func collectUniquePublicKeysFromUsers(log logr.Logger, sftpUserName string, users []sftpv1.User) ([]service.RoverPublicKeyModel, []sftpv1.InstanceUserStatus) {
	if len(users) == 0 {
		return []service.RoverPublicKeyModel{}, nil
	}

	fingerprints := map[string]*service.RoverPublicKeyModel{}
	keys := make([]service.RoverPublicKeyModel, 0, len(users))
	userStatuses := make([]sftpv1.InstanceUserStatus, 0, len(users))
	processingTime := v1.Now()
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
		userStatuses = append(userStatuses, newInstanceUserStatus(user, processingTime, hasInvalidSSHPublicKey))
	}

	return keys, userStatuses
}

func newInstanceUserStatus(user *sftpv1.User, processingTime v1.Time, hasInvalidSSHPublicKey bool) sftpv1.InstanceUserStatus {
	processing := condition.NewDoneProcessingCondition("SSH public keys were processed")
	if hasInvalidSSHPublicKey {
		processing = condition.NewBlockedCondition("Failed to process public key")
	}
	processing.ObservedGeneration = user.Generation
	processing.LastTransitionTime = processingTime
	return sftpv1.InstanceUserStatus{
		Namespace:           user.Namespace,
		Name:                user.Name,
		ProcessingCondition: processing,
	}
}

// preserveUserStatusesForSameGeneration keeps existing per-user status entries
// when the User generation has not changed, avoiding LastTransitionTime churn
// and unnecessary follow-up reconciliations.
func preserveUserStatusesForSameGeneration(current, next []sftpv1.InstanceUserStatus) {
	mapper := make(map[string]*sftpv1.InstanceUserStatus, len(current))
	for index := range current {
		mapper[current[index].Namespace+"/"+current[index].Name] = &current[index]
	}

	for index := range next {
		statusNew := &next[index]
		statusCurrent, ok := mapper[statusNew.Namespace+"/"+statusNew.Name]
		if !ok {
			continue
		}

		if statusNew.ProcessingCondition.ObservedGeneration == statusCurrent.ProcessingCondition.ObservedGeneration {
			statusCurrent.DeepCopyInto(statusNew)
		}
	}
}
