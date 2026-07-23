// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// HandleExposure creates or updates a file-domain FileExposure owned by the Rover.
func HandleExposure(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover, exp *roverv1.FileExposure) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Handle FileExposure", "fileType", exp.FileType)

	name := MakeName(exp.FileType, owner.Name)

	fileExposure := &filev1.FileExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: owner.Namespace,
		},
	}

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(owner, fileExposure, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		fileExposure.Labels = map[string]string{
			filev1.FileTypeLabelKey:             labelutil.NormalizeLabelValue(exp.FileType),
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}

		fileExposure.Spec = filev1.FileExposureSpec{
			Approval:   filev1.Approval{Strategy: filev1.ApprovalStrategy(exp.Approval.Strategy)},
			Visibility: filev1.Visibility(exp.Visibility.String()),
			FileType:   exp.FileType,
			Sftp: filev1.SftpExposure{
				PublicKeys: mapPublicKeys(exp.PublicKeys),
			},
			Zone: zoneRef,
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, fileExposure, mutator); err != nil {
		return errors.Wrap(err, "failed to create or update FileExposure")
	}

	owner.Status.FileExposures = append(owner.Status.FileExposures, types.ObjectRef{
		Name:      fileExposure.Name,
		Namespace: fileExposure.Namespace,
	})
	return nil
}
