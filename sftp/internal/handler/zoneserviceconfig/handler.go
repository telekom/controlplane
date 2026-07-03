// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zoneserviceconfig

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
)

var _ handler.Handler[*sftpv1.ZoneServiceConfig] = &ZoneServiceConfigHandler{}

type ZoneServiceConfigHandler struct {
	ClientManager service.ClientManager
}

func (h *ZoneServiceConfigHandler) CreateOrUpdate(ctx context.Context, obj *sftpv1.ZoneServiceConfig) error {
	log := logr.FromContextOrDiscard(ctx)
	isServiceCached := h.ClientManager.IsServiceCached(client.ObjectKeyFromObject(obj))

	conditionReady := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if isServiceCached && conditionReady != nil && conditionReady.ObservedGeneration == obj.Generation && conditionReady.Status == v1.ConditionTrue {
		log.V(1).Info("ZoneServiceConfig already reconciled")
		return nil
	}

	err := h.ClientManager.CreateOrUpdate(ctx, obj)
	if err != nil {
		return err
	}

	obj.SetCondition(condition.NewReadyCondition("ZoneServiceConfigProvided", "ZoneServiceConfig has been provided"))
	obj.SetCondition(condition.NewDoneProcessingCondition("ZoneServiceConfig has been provided"))
	return nil
}

func (h *ZoneServiceConfigHandler) Delete(ctx context.Context, obj *sftpv1.ZoneServiceConfig) error {
	h.ClientManager.Delete(obj)

	return nil
}
