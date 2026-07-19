// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

// HandleSubscription creates or updates an AgenticSubscription resource owned by the given Rover.
func HandleSubscription(ctx context.Context, c client.JanitorClient, owner *rover.Rover, sub *rover.AgenticSubscription) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handle AgenticSubscription", "basePath", sub.BasePath)

	name := MakeName(owner.Name, sub.BasePath)

	agenticSubscription := &agenticv1.AgenticSubscription{
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
		err := controllerutil.SetControllerReference(owner, agenticSubscription, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		agenticSubscription.Labels = map[string]string{
			agenticv1.AgenticBasePathLabelKey:   labelutil.NormalizeLabelValue(sub.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}

		agenticSubscription.Spec = agenticv1.AgenticSubscriptionSpec{
			BasePath: sub.BasePath,
			Zone:     zoneRef,
			Requestor: agenticv1.Requestor{
				Application: *owner.Status.Application,
			},
			Security: mapSubscriberSecurityToAgenticSecurity(sub.Security),
			Traffic:  mapSubscriberTrafficToAgenticTraffic(environment, sub.Traffic),
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, agenticSubscription, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update AgenticSubscription")
	}

	owner.Status.AgenticSubscriptions = append(owner.Status.AgenticSubscriptions, types.ObjectRef{
		Name:      agenticSubscription.Name,
		Namespace: agenticSubscription.Namespace,
	})
	return nil
}

// mapSubscriberSecurityToAgenticSecurity converts rover SubscriberSecurity to agentic SubscriberSecurity.
func mapSubscriberSecurityToAgenticSecurity(roverSecurity *rover.SubscriberSecurity) *agenticv1.SubscriberSecurity {
	if roverSecurity == nil {
		return nil
	}

	security := &agenticv1.SubscriberSecurity{}

	if roverSecurity.M2M != nil {
		security.M2M = &agenticv1.SubscriberMachine2MachineAuthentication{
			Scopes: roverSecurity.M2M.Scopes,
		}

		if roverSecurity.M2M.Client != nil {
			security.M2M.Client = &agenticv1.OAuth2ClientCredentials{
				ClientId:     roverSecurity.M2M.Client.ClientId,
				ClientSecret: roverSecurity.M2M.Client.ClientSecret,
				ClientKey:    roverSecurity.M2M.Client.ClientKey,
			}
		}

		if roverSecurity.M2M.Basic != nil {
			security.M2M.Basic = &agenticv1.BasicAuthCredentials{
				Username: roverSecurity.M2M.Basic.Username,
				Password: roverSecurity.M2M.Basic.Password,
			}
		}
	}

	return security
}

// mapSubscriberTrafficToAgenticTraffic converts rover SubscriberTraffic to agentic SubscriberTraffic.
func mapSubscriberTrafficToAgenticTraffic(env string, traffic rover.SubscriberTraffic) agenticv1.SubscriberTraffic {
	agenticTraffic := agenticv1.SubscriberTraffic{}

	if traffic.Failover != nil && traffic.Failover.Enabled {
		agenticTraffic.Failover = &agenticv1.SubscriberFailover{
			Enabled: true,
		}
	}

	return agenticTraffic
}
