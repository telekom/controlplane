// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func HandleApplication(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover) error {
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
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlerrors.BlockedErrorf("team not found for application %s", owner.Name)
		}
		return err
	}

	// If the Application publishes any events, we need to create a client for it, even if it doesn't have any subscriptions.
	// This is because the client is needed to access the publish-route
	hasAnyEventExposures := slices.ContainsFunc(owner.Spec.Exposures, func(ex roverv1.Exposure) bool {
		return ex.Type() == roverv1.TypeEvent
	})

	needsClient := len(owner.Spec.Subscriptions) > 0 || hasAnyEventExposures

	var hasAnySubscriptionFailoverEnabled bool
	if needsClient {
		for _, subscription := range owner.Spec.Subscriptions {
			switch subscription.Type() {
			case roverv1.TypeApi:
				failoverConfig := subscription.Api.Traffic.Failover
				if failoverConfig != nil && failoverConfig.Enabled {
					hasAnySubscriptionFailoverEnabled = true
					// break the inner loop, we only need to know if any subscription has failover enabled
					break
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
			Failover: applicationv1.Failover{
				Enabled: hasAnySubscriptionFailoverEnabled,
			},
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
