// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package sftpserviceconfig

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
)

var _ handler.Handler[*sftpv1.SFTPServiceConfig] = &SFTPServiceConfigHandler{}

type SFTPServiceConfigHandler struct {
	clientManager service.ClientManager
}

func New(clientManager service.ClientManager) (*SFTPServiceConfigHandler, error) {
	if clientManager == nil {
		return nil, errors.New("client manager is required")
	}

	return &SFTPServiceConfigHandler{
		clientManager: clientManager,
	}, nil
}

func (h *SFTPServiceConfigHandler) CreateOrUpdate(ctx context.Context, obj *sftpv1.SFTPServiceConfig) error {
	log := logf.FromContext(ctx)
	existClient := h.clientManager.ExistClient(client.ObjectKeyFromObject(obj))

	conditionReady := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if existClient && conditionReady != nil && conditionReady.ObservedGeneration == obj.Generation && conditionReady.Status == v1.ConditionTrue {
		log.V(1).Info("SFTPServiceConfig already reconciled")
		return nil
	}

	err := h.clientManager.CreateOrUpdate(ctx, obj)
	if err != nil {
		return err
	}

	obj.SetCondition(condition.NewReadyCondition("SFTPServiceConfigProvided", "SFTPServiceConfig has been provided"))
	obj.SetCondition(condition.NewDoneProcessingCondition("SFTPServiceConfig has been provided"))
	return nil
}

func (h *SFTPServiceConfigHandler) Delete(ctx context.Context, obj *sftpv1.SFTPServiceConfig) error {
	h.clientManager.Delete(obj)

	return nil
}
