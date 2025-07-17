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
			ApiBasePath:  sub.BasePath,
			Zone:         zoneRef,
			Security:     &apiapi.SubscriberSecurity{},
			Organization: sub.Organization,
			Requestor: apiapi.Requestor{
				Application: *owner.Status.Application,
			},
		}

		if sub.HasM2M() {
			apiSubscription.Spec.Security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{
				Client: toApiClient(sub.Security.M2M.Client),
				Basic:  toApiBasic(sub.Security.M2M.Basic),
				Scopes: sub.Security.M2M.Scopes,
			}
		}

		failoverZones, hasFailover := getFailoverZones(environment, sub.Traffic.Failover)
		if hasFailover {
			apiSubscription.Spec.Traffic = apiapi.Traffic{
				Failover: &apiapi.Failover{
					Zones: failoverZones,
				},
			}
		}

		apiSubscription.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeValue(sub.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, apiSubscription, mutator)
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
