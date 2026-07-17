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
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// HandleSubscription creates or updates a file-domain FileSubscription owned by the Rover.
func HandleSubscription(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover, sub *roverv1.FileSubscription) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Handle FileSubscription", "fileType", sub.FileType)

	name := MakeName(sub.FileType, owner.Name)

	fileSubscription := &filev1.FileSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: owner.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(owner, fileSubscription, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		fileSubscription.Labels = map[string]string{
			filev1.FileTypeLabelKey:             labelutil.NormalizeLabelValue(sub.FileType),
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(owner.Spec.Zone),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}

		fileSubscription.Spec = filev1.FileSubscriptionSpec{
			FileType: sub.FileType,
			Sftp: filev1.SftpSubscription{
				PublicKeys: mapPublicKeys(sub.PublicKeys),
			},
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, fileSubscription, mutator); err != nil {
		return errors.Wrap(err, "failed to create or update FileSubscription")
	}

	owner.Status.FileSubscriptions = append(owner.Status.FileSubscriptions, types.ObjectRef{
		Name:      fileSubscription.Name,
		Namespace: fileSubscription.Namespace,
	})
	return nil
}
