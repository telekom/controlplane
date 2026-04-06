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
			Name:      labelutil.NormalizeNameValue(name),
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
			Security:     mapSubscriberSecurityToApiSecurity(sub.Security),
			Organization: sub.Organization,
			Requestor: apiapi.Requestor{
				Application: *owner.Status.Application,
			},
		}

		// Handle failover configuration
		// Failover is ONLY enabled when failoverEnabled=true at rover spec level
		// Manual traffic.failover configuration is deprecated and blocked by validation
		if owner.Spec.FailoverEnabled {
			log.V(1).Info("FailoverEnabled is true, discovering DTC-eligible zones for subscription", "basePath", sub.BasePath)
			failoverZones, dtcErr := GetDtcEligibleZones(ctx, c, environment)
			if dtcErr != nil {
				return errors.Wrap(dtcErr, "failed to discover DTC-eligible zones")
			}

			if len(failoverZones) > 0 {
				apiSubscription.Spec.Traffic = apiapi.SubscriberTraffic{
					Failover: &apiapi.Failover{
						Zones: failoverZones,
					},
				}
				log.V(1).Info("Configured DTC failover zones for subscription", "basePath", sub.BasePath, "zones", failoverZones)
			} else {
				log.V(1).Info("No DTC-eligible zones found for failover", "basePath", sub.BasePath)
			}
		}
		// If failoverEnabled=false, no failover is configured (no traffic.failover at all)

		apiSubscription.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeLabelValue(sub.BasePath),
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

func mapSubscriberSecurityToApiSecurity(roverSecurity *rover.SubscriberSecurity) *apiapi.SubscriberSecurity {
	if roverSecurity == nil {
		return nil
	}

	security := &apiapi.SubscriberSecurity{}

	if roverSecurity.M2M != nil {
		security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{
			Client: toApiClient(roverSecurity.M2M.Client),
			Basic:  toApiBasic(roverSecurity.M2M.Basic),
			Scopes: roverSecurity.M2M.Scopes,
		}
	}

	return security
}
