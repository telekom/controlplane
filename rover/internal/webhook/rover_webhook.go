// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/types"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

// log is for logging in this package.
var roverlog = logf.Log.WithName("rover-webhook")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func SetupWebhookWithManager(mgr ctrl.Manager, secretManager secretsapi.SecretManager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&roverv1.Rover{}).
		WithDefaulter(&RoverDefaulter{mgr.GetClient(), secretManager}).
		WithValidator(&RoverValidator{mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-rover-cp-ei-telekom-de-v1-rover,mutating=true,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=rovers,verbs=create;update,versions=v1,name=mrover.kb.io,admissionReviewVersions=v1

type RoverDefaulter struct {
	client        client.Client
	secretManager secretsapi.SecretManager
}

var _ webhook.CustomDefaulter = &RoverDefaulter{}

func (r *RoverDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	roverlog.Info("default")
	rover, ok := obj.(*roverv1.Rover)
	if !ok {
		return apierrors.NewBadRequest("not a rover")
	}
	// No need to default if the object is being deleted
	if controller.IsBeingDeleted(rover) {
		return nil
	}
	ctx = logr.NewContext(ctx, roverlog.WithValues("name", rover.GetName(), "namespace", rover.GetNamespace()))
	err := OnboardApplication(ctx, rover, r.secretManager)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to onboard application: %w", err))
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-rover-cp-ei-telekom-de-v1-rover,mutating=false,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=rovers,verbs=create;update,versions=v1,name=vrover.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch

type RoverValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &RoverValidator{}

func (r *RoverValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	roverlog.Info("validate create")

	return r.ValidateCreateOrUpdate(ctx, obj)
}

func (r *RoverValidator) ValidateUpdate(ctx context.Context, oldObj, obj runtime.Object) (admission.Warnings, error) {
	roverlog.Info("validate update")

	return r.ValidateCreateOrUpdate(ctx, obj)
}

func (r *RoverValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	roverlog.Info("validate delete")

	return nil, nil // No validation needed on delete
}

func (r *RoverValidator) ValidateCreateOrUpdate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	rover, ok := obj.(*roverv1.Rover)
	if !ok {
		return nil, apierrors.NewBadRequest("not a rover")
	}

	log := roverlog.WithValues("name", rover.GetName(), "namespace", rover.GetNamespace())
	ctx = logr.NewContext(ctx, log)

	log.Info("validate create or update")

	valErr := cerrors.NewValidationError(roverv1.GroupVersion.WithKind("Rover").GroupKind(), rover)

	environment, ok := controller.GetEnvironment(rover)
	if !ok {
		valErr.AddInvalidError(cerrors.MetadataEnvPath, "", "environment label is required")
		return nil, valErr.BuildError()
	}

	zoneRef := client.ObjectKey{
		Name:      rover.Spec.Zone,
		Namespace: environment,
	}
	if exists, err := r.ResourceMustExist(ctx, zoneRef, &adminv1.Zone{}); !exists {
		if err != nil {
			return nil, err
		}
		valErr.AddInvalidError(field.NewPath("spec").Child("zone"), rover.Spec.Zone, fmt.Sprintf("zone '%s' not found", rover.Spec.Zone))
		return nil, valErr.BuildError()
	}

	if err := MustNotHaveDuplicates(valErr, rover.Spec.Subscriptions, rover.Spec.Exposures); err != nil {
		return nil, err
	}

	for i, sub := range rover.Spec.Subscriptions {
		log.Info("validate subscription", "index", i, "subscription", sub)
		if err := r.ValidateSubscription(ctx, valErr, environment, sub, i); err != nil {
			return nil, err
		}
	}

	for i, exposure := range rover.Spec.Exposures {
		log.Info("validate exposure", "index", i, "exposure", exposure)
		if err := r.ValidateExposure(ctx, valErr, environment, exposure, zoneRef, i); err != nil {
			return nil, err
		}
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}

func (r *RoverValidator) ResourceMustExist(ctx context.Context, objRef client.ObjectKey, obj client.Object) (bool, error) {
	err := r.client.Get(ctx, objRef, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, apierrors.NewInternalError(err)
	}
	return true, nil
}

func (r *RoverValidator) ValidateSubscription(ctx context.Context, valErr *cerrors.ValidationError, environment string, sub roverv1.Subscription, idx int) error {
	logr.FromContextOrDiscard(ctx).Info("validate subscription")

	if sub.Api != nil && sub.Api.Organization != "" {
		remoteOrgRef := client.ObjectKey{
			Name:      sub.Api.Organization,
			Namespace: environment,
		}
		if found, err := r.ResourceMustExist(ctx, remoteOrgRef, &adminv1.RemoteOrganization{}); !found {
			if err != nil {
				return err
			}
			valErr.AddInvalidError(
				field.NewPath("spec").Child("subscriptions").Index(idx).Child("api").Child("organization"),
				sub.Api.Organization, fmt.Sprintf("remote organization '%s' not found", sub.Api.Organization),
			)
		}
	}

	return nil
}

func (r *RoverValidator) ValidateExposure(ctx context.Context, valErr *cerrors.ValidationError, environment string, exposure roverv1.Exposure, zoneRef client.ObjectKey, idx int) error {
	if exposure.Api != nil {
		for _, upstream := range exposure.Api.Upstreams {
			if upstream.URL == "" {
				valErr.AddRequiredError(
					field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams").Index(0).Child("url"),
					"upstream URL must not be empty",
				)
				// Skip further URL validation if it's empty
				continue
			}
			if !strings.HasPrefix(upstream.URL, "http://") && !strings.HasPrefix(upstream.URL, "https://") {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams").Index(0).Child("url"),
					upstream.URL, "upstream URL must start with http:// or https://",
				)
			}
			if strings.Contains(upstream.URL, "localhost") {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams").Index(0).Child("url"),
					upstream.URL, "upstream URL must not contain 'localhost'",
				)
			}
		}

		// Validate rate limits if they are set
		if err := r.validateExposureRateLimit(ctx, valErr, exposure, idx); err != nil {
			return errors.Wrap(err, "failed to validate exposure rate limits")
		}

		// Check if all upstreams have a weight set or none
		all, none := CheckWeightSetOnAllOrNone(exposure.Api.Upstreams)
		if !all && !none {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams"),
				exposure.Api.Upstreams, "all upstreams must have a weight set or none must have a weight set",
			)
		}

		if err := r.validateApproval(ctx, valErr, environment, exposure.Api.Approval); err != nil {
			return errors.Wrap(err, "failed to validate approval")
		}

	}

	// Header removal is generally allowed everywhere, except the "Authorization" header, which is only allowed to be configured for removal on external zones - currently space/canis
	if err := r.validateRemoveHeaders(ctx, valErr, exposure, zoneRef, idx); err != nil {
		return err
	}

	return nil
}

func (r *RoverValidator) validateExposureRateLimit(ctx context.Context, valErr *cerrors.ValidationError, exposure roverv1.Exposure, idx int) error {
	// Check if API is nil
	if exposure.Api == nil {
		return nil
	}

	// Check if Traffic is nil
	if exposure.Api.Traffic == nil {
		return nil
	}

	// Check if RateLimit is nil
	if exposure.Api.Traffic.RateLimit == nil {
		return nil
	}

	// Validate provider rate limits
	if exposure.Api.Traffic.RateLimit.Provider != nil &&
		exposure.Api.Traffic.RateLimit.Provider.Limits != nil {
		if err := validateLimits(*exposure.Api.Traffic.RateLimit.Provider.Limits); err != nil {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("traffic").Child("rateLimit").Child("provider").Child("limits"),
				exposure.Api.Traffic.RateLimit.Provider.Limits, fmt.Sprintf("invalid provider rate limit: %v", err),
			)
		}
	}

	// Validate consumer rate limits
	if exposure.Api.Traffic.RateLimit.Consumers == nil {
		return nil
	}

	// Validate default consumer rate limit
	if exposure.Api.Traffic.RateLimit.Consumers.Default != nil {
		if err := validateLimits(exposure.Api.Traffic.RateLimit.Consumers.Default.Limits); err != nil {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("traffic").Child("rateLimit").Child("consumers").Child("default").Child("limits"),
				exposure.Api.Traffic.RateLimit.Consumers.Default.Limits, fmt.Sprintf("invalid default consumer rate limit: %v", err),
			)
		}
	}

	// Validate consumer overrides
	if exposure.Api.Traffic.RateLimit.Consumers.Overrides != nil {
		for i, override := range exposure.Api.Traffic.RateLimit.Consumers.Overrides {
			if err := validateLimits(override.Limits); err != nil {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("traffic").Child("rateLimit").Child("consumers").Child("overrides").Index(i).Child("limits"),
					override.Limits, fmt.Sprintf("invalid consumer override rate limit: %v", err),
				)
			}
		}
	}

	return nil
}

func (r *RoverValidator) validateApproval(ctx context.Context, valErr *cerrors.ValidationError, environment string, approval roverv1.Approval) error {

	for i := range approval.TrustedTeams {
		ref := types.ObjectRef{
			Name:      approval.TrustedTeams[i].Group + "--" + approval.TrustedTeams[i].Team,
			Namespace: environment,
		}
		if _, err := r.GetTeam(ctx, ref.K8s()); err != nil {
			if apierrors.IsNotFound(err) {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("exposures").Index(0).Child("api").Child("approval").Child("trustedTeams").Index(i),
					ref.Name, fmt.Sprintf("team '%s' not found", ref.Name),
				)
				continue
			}
			return errors.Wrap(err, "failed to get team")
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
func MustNotHaveDuplicates(valErr *cerrors.ValidationError, subs []roverv1.Subscription, exps []roverv1.Exposure) error {
	if len(subs) == 0 && len(exps) == 0 {
		return nil // No subscriptions or exposures, no duplicates to check
	}
	existingSubs := make(map[string]bool)
	for idx, sub := range subs {
		if sub.Api != nil {
			if _, exists := existingSubs[sub.Api.BasePath]; exists {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("subscriptions").Index(idx).Child("api").Child("basePath"),
					sub.Api.BasePath, fmt.Sprintf("duplicate subscription for base path %s", sub.Api.BasePath),
				)
			}
			existingSubs[sub.Api.BasePath] = true
		}

		if sub.Event != nil {
			if _, exists := existingSubs[sub.Event.EventType]; exists {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("subscriptions").Index(idx).Child("event").Child("eventType"),
					sub.Event.EventType, fmt.Sprintf("duplicate subscription for event-type %s", sub.Event.EventType),
				)
			}
			existingSubs[sub.Event.EventType] = true
		}
	}

	existingExps := make(map[string]bool)
	for idx, exposure := range exps {
		if exposure.Api != nil {
			if _, exists := existingExps[exposure.Api.BasePath]; exists {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("basePath"),
					exposure.Api.BasePath, fmt.Sprintf("duplicate exposure for base path %s", exposure.Api.BasePath),
				)
			}
			existingExps[exposure.Api.BasePath] = true
		}

		if exposure.Event != nil {
			if _, exists := existingExps[exposure.Event.EventType]; exists {
				valErr.AddInvalidError(
					field.NewPath("spec").Child("exposures").Index(idx).Child("event").Child("eventType"),
					exposure.Event.EventType, fmt.Sprintf("duplicate exposure for event-type %s", exposure.Event.EventType),
				)
			}
			existingExps[exposure.Event.EventType] = true
		}
	}

	return nil
}

func (r *RoverValidator) validateRemoveHeaders(ctx context.Context, valErr *cerrors.ValidationError, exp roverv1.Exposure, zoneRef client.ObjectKey, idx int) error {
	// Skip validation for event exposures
	if exp.Api == nil {
		return nil
	}

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
						valErr.AddInvalidError(
							field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("transformation").Child("request").Child("headers").Child("remove"),
							header, "removing 'Authorization' header is only allowed on external zones",
						)
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
			return nil, apierrors.NewBadRequest(fmt.Sprintf("Zone '%s' not found", zoneRef))
		}
		return nil, apierrors.NewInternalError(err)
	}
	return zone, nil

}

func (r *RoverValidator) GetTeam(ctx context.Context, teamRef client.ObjectKey) (*organizationv1.Team, error) {
	team := &organizationv1.Team{}
	err := r.client.Get(ctx, teamRef, team)
	return team, err

}
