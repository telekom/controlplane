// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"context"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
)

var _ handler.Handler[*sftpv1.Instance] = &InstanceHandler{}

type InstanceHandler struct {
	serviceFactory service.Factory
}

func New(serviceFactory service.Factory) (*InstanceHandler, error) {
	if serviceFactory == nil {
		return nil, errors.New("service factory is required")
	}

	return &InstanceHandler{
		serviceFactory: serviceFactory,
	}, nil
}

func (h *InstanceHandler) CreateOrUpdate(ctx context.Context, obj *sftpv1.Instance) error {
	if obj.Spec.SFTPServiceConfigRef.IsEmpty() {
		return ctrlerrors.BlockedErrorf("SFTPServiceConfig reference is required")
	}

	log := logf.FromContext(ctx)

	sftpService, err := h.serviceFactory.ServiceFor(ctx, obj.Spec.SFTPServiceConfigRef.K8s())
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
	obj.SetCondition(condition.NewReadyCondition("InstanceProvided", "Instance has been provided"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Instance has been provided"))
	return nil
}

func (h *InstanceHandler) Delete(ctx context.Context, obj *sftpv1.Instance) error {
	if obj.Spec.SFTPServiceConfigRef.IsEmpty() {
		return nil
	}

	sftpService, err := h.serviceFactory.ServiceFor(ctx, obj.Spec.SFTPServiceConfigRef.K8s())
	if err != nil {
		return err
	}

	err = sftpService.DeleteSFTPUser(ctx, obj.Name)
	if err != nil {
		return errors.Wrapf(err, "deleting SFTP user %q", obj.Name)
	}

	return nil
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
		return errors.Wrapf(err, "creating or updating SFTP user %q", instance.Name)
	}

	return nil
}
