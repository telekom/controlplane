// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// HandleSubscription creates or updates an EventSubscription resource owned by the given Rover.
func HandleSubscription(ctx context.Context, c client.JanitorClient, owner *rover.Rover, sub *rover.EventSubscription) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Handle EventSubscription", "eventType", sub.EventType)

	name := MakeName(owner.Name, sub.EventType)

	eventSubscription := &eventv1.EventSubscription{
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
		err := controllerutil.SetControllerReference(owner, eventSubscription, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		eventSubscription.Labels = map[string]string{
			eventv1.EventTypeLabelKey:           labelutil.NormalizeLabelValue(sub.EventType),
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}

		eventSubscription.Spec = eventv1.EventSubscriptionSpec{
			EventType: sub.EventType,
			Zone:      zoneRef,
			Requestor: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "application.cp.ei.telekom.de/v1",
				},
				ObjectRef: *owner.Status.Application,
			},
			Delivery: mapDelivery(sub.Delivery),
			Trigger:  mapEventTrigger(sub.Trigger),
			Scopes:   sub.Scopes,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, eventSubscription, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update EventSubscription")
	}

	owner.Status.EventSubscriptions = append(owner.Status.EventSubscriptions, types.ObjectRef{
		Name:      eventSubscription.Name,
		Namespace: eventSubscription.Namespace,
	})
	return nil
}

// mapDelivery converts a rover EventDelivery to an event-domain Delivery.
func mapDelivery(roverDelivery rover.EventDelivery) eventv1.Delivery {
	return eventv1.Delivery{
		Type:                  eventv1.DeliveryType(roverDelivery.Type),
		Payload:               eventv1.PayloadType(roverDelivery.Payload),
		Callback:              roverDelivery.Callback,
		EventRetentionTime:    roverDelivery.EventRetentionTime,
		CircuitBreakerOptOut:  roverDelivery.CircuitBreakerOptOut,
		RetryableStatusCodes:  roverDelivery.RetryableStatusCodes,
		RedeliveriesPerSecond: roverDelivery.RedeliveriesPerSecond,
		EnforceGetHttpRequestMethodForHealthCheck: roverDelivery.EnforceGetHttpRequestMethodForHealthCheck,
	}
}
