// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zoneserviceconfig

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

var _ handler.Handler[*filev1.ZoneServiceConfig] = &ZoneServiceConfigHandler{}

type ZoneServiceConfigHandler struct{}

func (h *ZoneServiceConfigHandler) CreateOrUpdate(ctx context.Context, obj *filev1.ZoneServiceConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)

	sftpConfig := &sftpv1.ZoneServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, sftpConfig, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		sftpConfig.Labels = util.ChildLabels(types.ObjectRef{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		})
		sftpConfig.Spec.API = obj.Spec.API
		sftpConfig.Spec.Service = obj.Spec.Service
		sftpConfig.Spec.ServiceExternal = obj.Spec.ServiceExternal
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, sftpConfig, mutator); err != nil {
		return errors.Wrapf(err, "failed to create or update SFTP ZoneServiceConfig %q", obj.Name)
	}

	obj.Status.SFTPZoneServiceConfigRef = types.ObjectRefFromObject(sftpConfig)

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady", "One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("ZoneServiceConfigProvisioned", "ZoneServiceConfig has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("ZoneServiceConfig has been provisioned"))
	return nil
}

func (h *ZoneServiceConfigHandler) Delete(ctx context.Context, obj *filev1.ZoneServiceConfig) error {
	return nil
}
