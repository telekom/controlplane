// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

// log is for logging in this package.
var roverlog = logf.Log.WithName("rover-webhook")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func SetupWebhookWithManager(mgr ctrl.Manager, secretManager secretsapi.SecretManager) error {
	return ctrl.NewWebhookManagedBy(mgr, &roverv1.Rover{}).
		WithDefaulter(&RoverDefaulter{mgr.GetClient(), secretManager}).
		WithValidator(&RoverValidator{mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-rover-cp-ei-telekom-de-v1-rover,mutating=true,failurePolicy=fail,sideEffects=None,groups=rover.cp.ei.telekom.de,resources=rovers,verbs=create;update,versions=v1,name=mrover.kb.io,admissionReviewVersions=v1

type RoverDefaulter struct {
	client        client.Client
	secretManager secretsapi.SecretManager
}

var _ admission.Defaulter[*roverv1.Rover] = &RoverDefaulter{}

func (r *RoverDefaulter) Default(ctx context.Context, rover *roverv1.Rover) error {
	roverlog.V(2).Info("default")
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
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=get;list;watch

type RoverValidator struct {
	client client.Client
}

var _ admission.Validator[*roverv1.Rover] = &RoverValidator{}

func (r *RoverValidator) ValidateCreate(ctx context.Context, rover *roverv1.Rover) (admission.Warnings, error) {
	roverlog.V(2).Info("validate create")

	return r.ValidateCreateOrUpdate(ctx, rover)
}

func (r *RoverValidator) ValidateUpdate(ctx context.Context, _ *roverv1.Rover, rover *roverv1.Rover) (admission.Warnings, error) {
	roverlog.V(2).Info("validate update")

	return r.ValidateCreateOrUpdate(ctx, rover)
}

func (r *RoverValidator) ValidateDelete(ctx context.Context, rover *roverv1.Rover) (admission.Warnings, error) {
	roverlog.V(2).Info("validate delete")

	return nil, nil // No validation needed on delete
}

func (r *RoverValidator) ValidateCreateOrUpdate(ctx context.Context, rover *roverv1.Rover) (admission.Warnings, error) {

	log := roverlog.WithValues("name", rover.GetName(), "namespace", rover.GetNamespace())
	ctx = logr.NewContext(ctx, log)

	log.V(2).Info("validate create or update")

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
	zone := &adminv1.Zone{}
	if exists, err := r.ResourceMustExist(ctx, zoneRef, zone); !exists {
		if err != nil {
			return nil, err
		}
		valErr.AddInvalidError(field.NewPath("spec").Child("zone"), rover.Spec.Zone, fmt.Sprintf("zone '%s' not found", rover.Spec.Zone))
		return nil, valErr.BuildError()
	}

	// Validate permissions: feature must be enabled if permissions are configured
	if !cconfig.FeaturePermission.IsEnabled() && len(rover.Spec.Permissions) > 0 {
		valErr.AddInvalidError(
			field.NewPath("spec").Child("zone"),
			rover.Spec.Zone,
			fmt.Sprintf("zone '%s' does not support permissions", rover.Spec.Zone))
		return nil, valErr.BuildError()
	}

	// Validate externalIds against the zone's policies.
	entries := make([]externalIdEntry, len(rover.Spec.ExternalIds))
	for i, eid := range rover.Spec.ExternalIds {
		entries[i] = externalIdEntry{Scheme: eid.Scheme, Id: eid.Id}
	}
	validateExternalIds(valErr, entries, zone, field.NewPath("spec").Child("externalIds"))

	// Validate permission structure: nested entries must have required fields based on parent format
	// This validation is done here in the webhook rather than via CEL in the CRD because CEL rules with
	// .all() iteration over the entries array would exceed the Kubernetes validation cost budget by
	// over 40x (even with MaxItems=50). Webhook validation has no such budget constraints.
	//
	// The validation ensures that permission entries will normalize into valid PermissionSet specs where
	// both role and resource are required fields:
	// - Resource-oriented format (resource + entries): each entry must have a non-empty role
	// - Role-oriented format (role + entries): each entry must have a non-empty resource
	// - Flat format (role + resource + actions): validated by existing CEL rules, no nested entries
	//
	// Without this validation, entries like {resource: "api", entries: [{actions: ["read"]}]} would
	// pass CRD validation but fail when the permission operator tries to create the PermissionSet CR.
	for i, perm := range rover.Spec.Permissions {
		permPath := field.NewPath("spec").Child("permissions").Index(i)

		// Resource-oriented: if resource is set, all entries must have role
		if perm.Resource != "" && len(perm.Entries) > 0 {
			for j, entry := range perm.Entries {
				if entry.Role == "" {
					valErr.AddInvalidError(
						permPath.Child("entries").Index(j).Child("role"),
						"",
						"role is required when parent permission has resource set (resource-oriented format)")
				}
			}
		}

		// Role-oriented: if role is set, all entries must have resource
		if perm.Role != "" && len(perm.Entries) > 0 {
			for j, entry := range perm.Entries {
				if entry.Resource == "" {
					valErr.AddInvalidError(
						permPath.Child("entries").Index(j).Child("resource"),
						"",
						"resource is required when parent permission has role set (role-oriented format)")
				}
			}
		}
	}

	// Validate that if the rover subscribes to or exposes events, the zone actually supports it
	subscribesToEvents := slices.ContainsFunc(rover.Spec.Subscriptions, func(sub roverv1.Subscription) bool {
		return sub.Type() == roverv1.TypeEvent
	})
	exposesEvents := slices.ContainsFunc(rover.Spec.Exposures, func(exp roverv1.Exposure) bool {
		return exp.Type() == roverv1.TypeEvent
	})

	if cconfig.FeaturePubSub.IsEnabled() && (subscribesToEvents || exposesEvents) {
		eventConfigRef := client.ObjectKey{
			Name:      environment,
			Namespace: zone.Status.Namespace,
		}
		eventConfig := eventv1.EventConfig{}
		if exists, err := r.ResourceMustExist(ctx, eventConfigRef, &eventConfig); !exists {
			if err != nil {
				return nil, err
			}
			valErr.AddInvalidError(field.NewPath("spec").Child("zone"), rover.Spec.Zone, fmt.Sprintf("zone '%s' does not support event subscriptions or exposures", rover.Spec.Zone))
		}
	}

	if err := MustNotHaveDuplicates(valErr, rover.Spec.Subscriptions, rover.Spec.Exposures); err != nil {
		return nil, err
	}

	for i, sub := range rover.Spec.Subscriptions {
		log.V(2).Info("validate subscription", "index", i, "subscription", sub)
		if err := r.ValidateSubscription(ctx, valErr, environment, sub, i); err != nil {
			return nil, err
		}
	}

	for i, exposure := range rover.Spec.Exposures {
		log.V(2).Info("validate exposure", "index", i, "exposure", exposure)
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

func (r *RoverValidator) ValidateExposure(ctx context.Context, valErr *cerrors.ValidationError, environment string, exposure roverv1.Exposure, zoneRef client.ObjectKey, idx int) error {

	switch exposure.Type() {
	case roverv1.TypeApi:
		return r.ValidateApiExposure(ctx, valErr, environment, exposure, zoneRef, idx)
	case roverv1.TypeEvent:
		return r.ValidateEventExposure(ctx, valErr, environment, exposure, zoneRef, idx)
	default:
		valErr.AddInvalidError(
			field.NewPath("spec").Child("exposures").Index(idx).Child("type"),
			exposure.Type(), fmt.Sprintf("unknown exposure type %q", exposure.Type()),
		)
		return nil
	}
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

func (r *RoverValidator) ValidateEventExposure(ctx context.Context, valErr *cerrors.ValidationError, environment string, exposure roverv1.Exposure, zoneRef client.ObjectKey, idx int) error {
	if exposure.Event == nil {
		return nil
	}

	if !cconfig.FeaturePubSub.IsEnabled() {
		return nil
	}

	if err := r.validateApproval(ctx, valErr, environment, exposure.Event.Approval); err != nil {
		return errors.Wrap(err, "failed to validate approval")
	}

	return nil
}

func (r *RoverValidator) ValidateApiExposure(ctx context.Context, valErr *cerrors.ValidationError, environment string, exposure roverv1.Exposure, zoneRef client.ObjectKey, idx int) error {
	if exposure.Api == nil {
		return nil
	}

	for i, upstream := range exposure.Api.Upstreams {
		if upstream.URL == "" {
			valErr.AddRequiredError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams").Index(i).Child("url"),
				"upstream URL must not be empty",
			)
			// Skip further URL validation if it's empty
			continue
		}
		if !strings.HasPrefix(upstream.URL, "http://") && !strings.HasPrefix(upstream.URL, "https://") {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams").Index(i).Child("url"),
				upstream.URL, "upstream URL must start with http:// or https://",
			)
		}
		if strings.Contains(upstream.URL, "localhost") {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams").Index(i).Child("url"),
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

	// Header removal is generally allowed everywhere, except the "Authorization" header, which is only allowed to be configured for removal on external zones - currently space/canis
	if err := r.validateRemoveHeaders(ctx, valErr, exposure, zoneRef, idx); err != nil {
		return err
	}

	return nil
}

func (r *RoverValidator) ValidateSubscription(ctx context.Context, valErr *cerrors.ValidationError, environment string, sub roverv1.Subscription, idx int) error {
	switch sub.Type() {
	case roverv1.TypeApi:
		// TODO: in the future this might also be relevant for event
		if sub.Api.Organization != "" {
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

	case roverv1.TypeEvent:
		// There is no special validation needed at this time.
		return nil
	}

	return nil
}
