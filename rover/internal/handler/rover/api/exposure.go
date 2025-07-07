// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	rover "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func HandleExposure(ctx context.Context, c client.JanitorClient, owner *rover.Rover, exp *rover.ApiExposure) error {

	log := log.FromContext(ctx)
	log.V(1).Info("Handle APIExposure", "basePath", exp.BasePath)

	name := MakeName(owner.Name, exp.BasePath, "")

	apiExposure := &apiapi.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Namespace,
		},
	}

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	mutator := func() error {
		err := controllerutil.SetControllerReference(owner, apiExposure, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}
		apiExposure.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeValue(exp.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
		}

		apiExposure.Spec = apiapi.ApiExposureSpec{
			ApiBasePath: exp.BasePath,
			Visibility:  apiapi.Visibility(exp.Visibility.String()),
			Approval:    apiapi.ApprovalStrategy(exp.Approval.Strategy),
			Zone:        zoneRef,
			Upstreams: []apiapi.Upstream{
				{
					Url:    exp.Upstreams[0].URL,
					Weight: 100,
				},
			},
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, apiExposure, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update ApiExposure")
	}

	owner.Status.ApiExposures = append(owner.Status.ApiExposures, types.ObjectRef{
		Name:      apiExposure.Name,
		Namespace: apiExposure.Namespace,
	})
	return err
}
