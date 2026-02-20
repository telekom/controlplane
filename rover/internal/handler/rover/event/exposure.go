// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// HandleExposure creates or updates an EventExposure resource owned by the given Rover.
func HandleExposure(ctx context.Context, c client.JanitorClient, owner *rover.Rover, exp *rover.EventExposure) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Handle EventExposure", "eventType", exp.EventType)

	name := MakeName(owner.Name, exp.EventType)

	eventExposure := &eventv1.EventExposure{
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
		err := controllerutil.SetControllerReference(owner, eventExposure, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		eventExposure.Labels = map[string]string{
			eventv1.EventTypeLabelKey:           labelutil.NormalizeLabelValue(exp.EventType),
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}

		// Map trusted teams from rover Group/Team format to resolved team names
		trustedTeams, err := mapTrustedTeams(ctx, c, exp.Approval.TrustedTeams)
		if err != nil {
			return errors.Wrap(err, "failed to map trusted teams")
		}

		// Add owner team to trusted teams
		ownerTeam, err := organizationv1.FindTeamForObject(ctx, owner)
		if err != nil && apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Team not found for application %s, err: %v", owner.Name, err))
		} else if err != nil {
			return err
		} else {
			trustedTeams = append(trustedTeams, ownerTeam.GetName())
		}

		eventExposure.Spec = eventv1.EventExposureSpec{
			EventType:  exp.EventType,
			Visibility: eventv1.Visibility(exp.Visibility.String()),
			Approval: eventv1.Approval{
				Strategy:     eventv1.ApprovalStrategy(exp.Approval.Strategy),
				TrustedTeams: trustedTeams,
			},
			Zone: zoneRef,
			Provider: types.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "application.cp.ei.telekom.de/v1",
				},
				ObjectRef: *owner.Status.Application,
			},
			Scopes:                 mapEventScopes(exp.Scopes),
			AdditionalPublisherIds: exp.AdditionalPublisherIds,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, eventExposure, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update EventExposure")
	}

	owner.Status.EventExposures = append(owner.Status.EventExposures, types.ObjectRef{
		Name:      eventExposure.Name,
		Namespace: eventExposure.Namespace,
	})
	return nil
}

// mapTrustedTeams resolves rover TrustedTeam references (Group/Team) to team resource names.
func mapTrustedTeams(ctx context.Context, c client.JanitorClient, teams []rover.TrustedTeam) ([]string, error) {
	log := log.FromContext(ctx)
	if len(teams) == 0 {
		return nil, nil
	}

	resolved := make([]string, 0, len(teams))
	for _, team := range teams {
		namespace := contextutil.EnvFromContextOrDie(ctx) + "--" + team.Group + "--" + team.Team
		t, err := organizationv1.FindTeamForNamespace(ctx, namespace)
		if err != nil && apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Trusted team %s/%s not found", team.Group, team.Team))
		} else if err != nil {
			return nil, err
		} else {
			resolved = append(resolved, t.GetName())
		}
	}

	return resolved, nil
}

// mapEventScopes converts rover EventScope types to event-domain EventScope types.
func mapEventScopes(roverScopes []rover.EventScope) []eventv1.EventScope {
	if len(roverScopes) == 0 {
		return nil
	}

	scopes := make([]eventv1.EventScope, len(roverScopes))
	for i, s := range roverScopes {
		scopes[i] = eventv1.EventScope{
			Name:    s.Name,
			Trigger: mapEventTriggerValue(s.Trigger),
		}
	}
	return scopes
}

// mapEventTrigger converts a rover EventTrigger pointer to an event-domain EventTrigger pointer.
// Used for subscriber-side triggers where the trigger is optional.
func mapEventTrigger(roverTrigger *rover.EventTrigger) *eventv1.EventTrigger {
	if roverTrigger == nil {
		return nil
	}

	trigger := &eventv1.EventTrigger{}

	if roverTrigger.ResponseFilter != nil {
		trigger.ResponseFilter = &eventv1.ResponseFilter{
			Paths: roverTrigger.ResponseFilter.Paths,
			Mode:  eventv1.ResponseFilterMode(roverTrigger.ResponseFilter.Mode),
		}
	}

	if roverTrigger.SelectionFilter != nil {
		trigger.SelectionFilter = &eventv1.SelectionFilter{
			Attributes: roverTrigger.SelectionFilter.Attributes,
			Expression: roverTrigger.SelectionFilter.Expression,
		}
	}

	return trigger
}

// mapEventTriggerValue converts a rover EventTrigger value to an event-domain EventTrigger value.
// Used for scope triggers where the trigger is required.
func mapEventTriggerValue(roverTrigger rover.EventTrigger) eventv1.EventTrigger {
	trigger := eventv1.EventTrigger{}

	if roverTrigger.ResponseFilter != nil {
		trigger.ResponseFilter = &eventv1.ResponseFilter{
			Paths: roverTrigger.ResponseFilter.Paths,
			Mode:  eventv1.ResponseFilterMode(roverTrigger.ResponseFilter.Mode),
		}
	}

	if roverTrigger.SelectionFilter != nil {
		trigger.SelectionFilter = &eventv1.SelectionFilter{
			Attributes: roverTrigger.SelectionFilter.Attributes,
			Expression: roverTrigger.SelectionFilter.Expression,
		}
	}

	return trigger
}
