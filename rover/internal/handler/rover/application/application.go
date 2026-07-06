// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"fmt"
	"slices"

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
	// If the Application publishes any events, we need to create a client for it, even if it doesn't have any subscriptions.
	// This is because the client is needed to access the publish-route
	needsClient := RoverNeedsClient(owner)
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

		// Preserve existing Application secret on updates (write-once);
		// only bootstrap from Rover on initial creation.
		secretToApply := application.Spec.Secret
		if secretToApply == "" {
			secretToApply = owner.Spec.ClientSecret
		}

		application.Spec = applicationv1.ApplicationSpec{
			Team:          team.Name,
			TeamEmail:     team.Spec.Email,
			Zone:          zoneRef,
			NeedsClient:   needsClient,
			NeedsConsumer: needsClient,
			Secret:        secretToApply,
			FailoverZones: subscriberFailoverZones,
			RotatedSecret: application.Spec.RotatedSecret,
		}

		if owner.Spec.IpRestrictions != nil {
			application.Spec.Security = &applicationv1.Security{
				IpRestrictions: &applicationv1.IpRestrictions{
					Allow: owner.Spec.IpRestrictions.Allow,
					Deny:  owner.Spec.IpRestrictions.Deny,
				},
			}
		}

		if len(owner.Spec.ExternalIds) > 0 {
			application.Spec.ExternalIds = make([]applicationv1.ExternalId, len(owner.Spec.ExternalIds))
			for i, eid := range owner.Spec.ExternalIds {
				application.Spec.ExternalIds[i] = applicationv1.ExternalId{Scheme: eid.Scheme, Id: eid.Id}
			}
		} else {
			application.Spec.ExternalIds = nil
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

// RoverNeedsClient reports whether the Application derived from the given Rover
// requires an Identity client (and Gateway consumer).
//
// File-type (SFTP) subscriptions and exposures are realized in the file domain via
// SFTP users and SSH public keys. They neither need an Identity client nor a Gateway
// consumer. Only non-file subscriptions or any event exposure force a client/consumer.
func RoverNeedsClient(owner *roverv1.Rover) bool {
	hasAnyEventExposures := slices.ContainsFunc(owner.Spec.Exposures, func(ex roverv1.Exposure) bool {
		return ex.Type() == roverv1.TypeEvent
	})
	hasNonFileSubscriptions := slices.ContainsFunc(owner.Spec.Subscriptions, func(sub roverv1.Subscription) bool {
		return sub.Type() != roverv1.TypeFile
	})
	return hasNonFileSubscriptions || hasAnyEventExposures
}
