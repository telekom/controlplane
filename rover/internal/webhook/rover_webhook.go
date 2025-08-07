// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/types"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var roverlog = logf.Log.WithName("rover-webhook")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&roverv1.Rover{}).
		WithDefaulter(&RoverDefaulter{mgr.GetClient()}).
		WithValidator(&RoverValidator{mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-rover-cp-ei-telekom-de-v1-rover,mutating=true,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=rovers,verbs=create;update,versions=v1,name=mrover.kb.io,admissionReviewVersions=v1

type RoverDefaulter struct {
	client client.Client
}

var _ webhook.CustomDefaulter = &RoverDefaulter{}

func (r *RoverDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	roverlog.Info("default")

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-rover,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=rovers,verbs=create;update,versions=v1,name=vrover.kb.io,admissionReviewVersions=v1

// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch

type RoverValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &RoverValidator{}

func (r *RoverValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	roverlog.Info("validate create")

	return r.ValidateCreateOrUpdate(ctx, obj)
}

func (r *RoverValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (warnings admission.Warnings, err error) {
	roverlog.Info("validate update")

	return r.ValidateCreateOrUpdate(ctx, obj)
}

func (r *RoverValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	roverlog.Info("validate delete")

	return
}

func (r *RoverValidator) ValidateCreateOrUpdate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	rover, ok := obj.(*roverv1.Rover)
	if !ok {
		return nil, apierrors.NewBadRequest("not a rover")
	}

	roverlog.Info("validate create or update", "name", rover.GetName())

	environment, ok := controller.GetEnvironment(rover)
	if !ok {
		return nil, apierrors.NewBadRequest("environment not found")
	}

	zoneRef := client.ObjectKey{
		Name:      rover.Spec.Zone,
		Namespace: environment,
	}
	if err = r.ResourceMustExist(ctx, zoneRef, &adminv1.Zone{}); err != nil {
		return nil, err
	}

	if err := MustNotHaveDuplicates(rover.Spec.Subscriptions, rover.Spec.Exposures); err != nil {
		return nil, err
	}

	for _, sub := range rover.Spec.Subscriptions {
		if _, err = r.ValidateSubscription(ctx, environment, sub); err != nil {
			return nil, err
		}
	}

	for _, exposure := range rover.Spec.Exposures {
		if _, err = r.ValidateExposure(ctx, environment, exposure, zoneRef); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (r *RoverValidator) ResourceMustExist(ctx context.Context, objRef client.ObjectKey, obj client.Object) error {
	err := r.client.Get(ctx, objRef, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewBadRequest(fmt.Sprintf("%s not found", reflect.TypeOf(obj).Elem().Name()))
		}
		return apierrors.NewInternalError(err)
	}
	return nil
}

func (r *RoverValidator) ValidateSubscription(ctx context.Context, environment string, sub roverv1.Subscription) (warnings admission.Warnings, err error) {
	roverlog.Info("validate subscription")

	if sub.Api != nil && sub.Api.Organization != "" {
		remoteOrgRef := client.ObjectKey{
			Name:      sub.Api.Organization,
			Namespace: environment,
		}
		if err = r.ResourceMustExist(ctx, remoteOrgRef, &adminv1.RemoteOrganization{}); err != nil {
			return nil, err
		}
	}

	return
}

func (r *RoverValidator) ValidateExposure(ctx context.Context, environment string, exposure roverv1.Exposure, zoneRef client.ObjectKey) (warnings admission.Warnings, err error) {
	if exposure.Api != nil {
		for _, upstream := range exposure.Api.Upstreams {
			if upstream.URL == "" {
				return nil, apierrors.NewBadRequest("upstream URL must not be empty")
			}
			if !strings.HasPrefix(upstream.URL, "http://") && !strings.HasPrefix(upstream.URL, "https://") {
				return nil, apierrors.NewBadRequest("upstream URL must start with http:// or https://")
			}
			if strings.Contains(upstream.URL, "localhost") {
				return nil, apierrors.NewBadRequest("upstream URL must not contain localhost")
			}
		}

		// Validate rate limit configuration if present
		if exposure.Api.Traffic != nil {
			if exposure.Api.Traffic.HasRateLimit() {
				// Validate provider rate limits
				if exposure.Api.Traffic.HasProviderRateLimitLimits() {
					if err := validateLimits(*exposure.Api.Traffic.RateLimit.Provider.Limits); err != nil {
						return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid provider rate limit: %v", err))
					}
				}

				// Validate consumer rate limits
				if exposure.Api.Traffic.HasConsumerRateLimit() {
					// Validate default consumer rate limit
					if exposure.Api.Traffic.HasConsumerDefaultsRateLimit() {
						if err := validateLimits(exposure.Api.Traffic.RateLimit.Consumers.Default.Limits); err != nil {
							return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid default consumer rate limit: %v", err))
						}
					}

					// Validate consumer overrides
					if exposure.Api.Traffic.HasConsumerOverridesRateLimit() {
						for _, override := range exposure.Api.Traffic.RateLimit.Consumers.Overrides {
							if err := validateLimits(override.Limits); err != nil {
								return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid consumer override rate limit for consumer %s: %v", override.Consumer, err))
							}
						}
					}
				}
			}
		}
	}

	// Check if all upstreams have a weight set or none
	all, none := CheckWeightSetOnAllOrNone(exposure.Api.Upstreams)
	if !all && !none {
		return nil, apierrors.NewBadRequest("all upstreams must have a weight set or none must have a weight set")
	}

	// Header removal is generally allowed everywhere, except the "Authorization" header, which is only allowed to be configured for removal on external zones - currently space/canis
	if err = r.validateRemoveHeaders(ctx, exposure, zoneRef); err != nil {
		return nil, err
	}

	//
	if err = r.validateApproval(ctx, environment, exposure.Api.Approval); err != nil {
		return nil, err
	}

	return
}

func (r *RoverValidator) validateApproval(ctx context.Context, environment string, approval roverv1.Approval) error {

	for i := range approval.TrustedTeams {
		ref := types.ObjectRef{
			Name:      approval.TrustedTeams[i].Group + "--" + approval.TrustedTeams[i].Team,
			Namespace: environment,
		}
		if _, err := r.GetTeam(ctx, ref.K8s()); err != nil {
			return err
		}
	}

	return nil
}

func validateLimits(limits roverv1.Limits) error {
	// Check if at least one time window is specified
	if limits.Second == 0 && limits.Minute == 0 && limits.Hour == 0 {
		return fmt.Errorf("at least one of second, minute, or hour must be specified")
	}

	// Check that second < minute if both are specified
	if limits.Second > 0 && limits.Minute > 0 && limits.Second >= limits.Minute {
		return fmt.Errorf("second (%d) must be less than minute (%d)", limits.Second, limits.Minute)
	}

	// Check that minute < hour if both are specified
	if limits.Minute > 0 && limits.Hour > 0 && limits.Minute >= limits.Hour {
		return fmt.Errorf("minute (%d) must be less than hour (%d)", limits.Minute, limits.Hour)
	}

	return nil
}

func CheckWeightSetOnAllOrNone(upstreams []roverv1.Upstream) (allSet, noneSet bool) {
	if len(upstreams) == 0 {
		return true, true
	}

	allSet = true
	noneSet = true

	for _, upstream := range upstreams {
		// In Go, with `omitempty` and a non-pointer `int`, if the field is omitted in JSON,
		// it will be unmarshalled as `0`.
		if upstream.Weight == 0 {
			allSet = false
		} else {
			noneSet = false
		}
	}

	return allSet, noneSet
}

// MustNotHaveDuplicates checks if there are no duplicates in the subscriptions and exposures
func MustNotHaveDuplicates(subs []roverv1.Subscription, exps []roverv1.Exposure) error {
	exisitingSubs := make(map[string]bool)
	for _, sub := range subs {
		if sub.Api != nil {
			if _, exists := exisitingSubs[sub.Api.BasePath]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate subscription for base path %s", sub.Api.BasePath))
			}
			exisitingSubs[sub.Api.BasePath] = true
		}

		if sub.Event != nil {
			if _, exists := exisitingSubs[sub.Event.EventType]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate subscription for event-type %s", sub.Event.EventType))
			}
			exisitingSubs[sub.Event.EventType] = true
		}
	}

	exisitingExps := make(map[string]bool)
	for _, exposure := range exps {
		if exposure.Api != nil {
			if _, exists := exisitingExps[exposure.Api.BasePath]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate exposure for base path %s", exposure.Api.BasePath))
			}
			exisitingExps[exposure.Api.BasePath] = true
		}

		if exposure.Event != nil {
			if _, exists := exisitingExps[exposure.Event.EventType]; exists {
				return apierrors.NewBadRequest(fmt.Sprintf("duplicate exposure for event-type %s", exposure.Event.EventType))
			}
			exisitingExps[exposure.Event.EventType] = true
		}
	}

	return nil
}

func (r *RoverValidator) validateRemoveHeaders(ctx context.Context, exp roverv1.Exposure, zoneRef client.ObjectKey) error {

	// get zone
	zone, err := r.GetZone(ctx, zoneRef)
	if err != nil {
		return err
	}
	if exp.Api.Transformation != nil {
		if len(exp.Api.Transformation.Request.Headers.Remove) > 0 {
			for _, header := range exp.Api.Transformation.Request.Headers.Remove {
				if strings.EqualFold(header, "Authorization") {
					if zone.Spec.Visibility != adminv1.ZoneVisibilityWorld {
						return apierrors.NewBadRequest("removal of 'Authorization' header is only allowed for external zones, i.e space or canis")
					}
				}
			}
		}
	}

	return nil
}

func (r *RoverValidator) GetZone(ctx context.Context, zoneRef client.ObjectKey) (*adminv1.Zone, error) {
	zone := &adminv1.Zone{}
	err := r.client.Get(ctx, zoneRef, zone)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("%s not found", reflect.TypeOf(zone).Elem().Name()))
		}
		return nil, apierrors.NewInternalError(err)
	}
	return zone, nil

}

func (r *RoverValidator) GetTeam(ctx context.Context, teamRef client.ObjectKey) (*organizationv1.Team, error) {
	team := &organizationv1.Team{}
	err := r.client.Get(ctx, teamRef, team)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("%s not found", reflect.TypeOf(team).Elem().Name()))
		}
		return nil, apierrors.NewInternalError(err)
	}
	return team, nil

}
