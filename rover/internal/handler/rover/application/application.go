// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func HandleApplication(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover) error {
	log := log.FromContext(ctx)
	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	application := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name,
			Namespace: owner.Namespace,
		},
	}

	team, err := organizationv1.FindTeamForObject(ctx, owner)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info(fmt.Sprintf("Team not found for application %s, err: %v", owner.Name, err))
	} else if err != nil {
		return err
	}

	needsClient := len(owner.Spec.Subscriptions) > 0
	var subscriberFailoverZones []types.ObjectRef
	if needsClient {
		for _, subscription := range owner.Spec.Subscriptions {
			switch subscription.Type() {
			case roverv1.TypeApi:
				if subscription.Api.Traffic.Failover != nil {
					for _, zoneName := range subscription.Api.Traffic.Failover.Zones {
						zoneRef := types.ObjectRef{
							Name:      zoneName,
							Namespace: environment,
						}
						subscriberFailoverZones = append(subscriberFailoverZones, zoneRef)
					}
				}
			}
		}
	}

	mutator := func() error {
		application.Labels = map[string]string{
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
			config.BuildLabelKey("team"):        labelutil.NormalizeValue(team.Name),
		}

		err := controllerutil.SetControllerReference(owner, application, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		application.Spec = applicationv1.ApplicationSpec{
			Team:          team.Name,
			TeamEmail:     team.Spec.Email,
			Zone:          zoneRef,
			NeedsClient:   needsClient,
			NeedsConsumer: needsClient,
			Secret:        owner.Spec.ClientSecret,
			FailoverZones: subscriberFailoverZones,
		}

		if owner.Spec.IpRestrictions != nil {
			application.Spec.Security = &applicationv1.Security{
				IpRestrictions: &applicationv1.IpRestrictions{
					Allow: owner.Spec.IpRestrictions.Allow,
					Deny:  owner.Spec.IpRestrictions.Deny,
				},
			}
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, application, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update application")
	}

	owner.Status.Application = &types.ObjectRef{
		Name:      application.Name,
		Namespace: application.Namespace,
	}

	return err
}
