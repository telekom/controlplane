// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"github.com/telekom/controlplane/common/pkg/condition"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

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

func HandleSubscription(ctx context.Context, c client.JanitorClient, owner *rover.Rover, sub *rover.ApiSubscription) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Handle APISubscription", "basePath", sub.BasePath)

	name := MakeName(owner.Name, sub.BasePath, sub.Organization)

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	apiSubscription := &apiapi.ApiSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Namespace,
		},
	}

	mutator := func() error {
		err := controllerutil.SetControllerReference(owner, apiSubscription, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		apiSubscription.Spec = apiapi.ApiSubscriptionSpec{
			ApiBasePath: sub.BasePath,
			Zone:        zoneRef,
			Security: &apiapi.Security{
				Oauth2Scopes: sub.OAuth2Scopes,
			},
			Organization: sub.Organization,
			Requestor: apiapi.Requestor{
				Application: *owner.Status.Application,
			},
		}

		apiSubscription.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeValue(sub.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, apiSubscription, mutator)

	// many different errors can occur, so lets handle them
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		if statusErr.ErrStatus.Reason == metav1.StatusReasonBadRequest {
			errorMessage := "Create or update ApiSubscription failed. Webhook validation error - BadRequest."
			log.V(0).Error(err, errorMessage)
			return errors.Wrap(err, errorMessage)
		} else if statusErr.ErrStatus.Reason == metav1.StatusReasonNotFound {
			errorMessage := "Create or update ApiSubscription failed. Webhook validation error - NotFound."
			log.V(0).Error(err, errorMessage)
			owner.SetCondition(condition.NewBlockedCondition("Blocked due to missing ApiExposure for subscription to basepath '" + sub.BasePath + "'"))
			return errors.Wrap(err, errorMessage)
		}
	} else if err != nil {
		return errors.Wrap(err, "failed to create or update ApiSubscription")
	}

	owner.Status.ApiSubscriptions = append(owner.Status.ApiSubscriptions, types.ObjectRef{
		Name:      apiSubscription.Name,
		Namespace: apiSubscription.Namespace,
	})

	log.V(1).Info("Created ApiSubscription", "subscription", apiSubscription)

	return err
}
