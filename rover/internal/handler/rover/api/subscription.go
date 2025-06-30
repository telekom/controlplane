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

func HandleSubscription(ctx context.Context, c client.JanitorClient, owner *rover.Rover, sub *rover.ApiSubscription) error {

	log := log.FromContext(ctx)
	log.V(1).Info("Handle APISubscription", "basePath", sub.BasePath)

	apiSubscription, mutator, err := BuildApiSubscription(ctx, c, *owner, *sub)
	if err != nil {
		return errors.Wrap(err, "failed to build ApiSubscription and its mutator function")
	}

	_, err = c.CreateOrUpdate(ctx, apiSubscription, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update ApiSubscription")
	}

	owner.Status.ApiSubscriptions = append(owner.Status.ApiSubscriptions, types.ObjectRef{
		Name:      apiSubscription.Name,
		Namespace: apiSubscription.Namespace,
	})

	log.V(1).Info("Created ApiSubscription", "subscription", apiSubscription)

	return err
}

func BuildApiSubscription(ctx context.Context, c client.ScopedClient, owner rover.Rover, sub rover.ApiSubscription) (*apiapi.ApiSubscription, controllerutil.MutateFn, error) {
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
		err := controllerutil.SetControllerReference(&owner, apiSubscription, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		// when called from a webhook, the status is not present yet, so omit those parts
		var applicationRef types.ObjectRef
		if owner.Status.IsEmpty() {
			applicationRef = types.ObjectRef{}
		} else {
			applicationRef = *owner.Status.Application
		}
		apiSubscription.Spec = apiapi.ApiSubscriptionSpec{
			ApiBasePath: sub.BasePath,
			Zone:        zoneRef,
			Security: &apiapi.Security{
				Oauth2Scopes: sub.OAuth2Scopes,
			},
			Organization: sub.Organization,
			Requestor: apiapi.Requestor{
				Application: applicationRef,
			},
		}

		apiSubscription.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeValue(sub.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
			// when called from a webhook this needs to be added manually - normally done by the scoped client
			config.EnvironmentLabelKey: environment,
		}
		return nil
	}

	return apiSubscription, mutator, nil
}
