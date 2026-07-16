// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
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

func (r *RoverValidator) ValidateUpdate(ctx context.Context, _, rover *roverv1.Rover) (admission.Warnings, error) {
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

	zoneRef, zone, err := r.validateZone(ctx, valErr, rover, environment)
	if err != nil {
		return nil, err
	}
	if zone == nil {
		return nil, valErr.BuildError()
	}

	// Validate ConsumerFailover: feature must be enabled on this zone if ConsumerFailover is configured
	if !zone.IsFeatureEnabled(adminv1.FeatureConsumerFailover) && rover.HasFailoverEnabledOnAnySubscription() {
		valErr.AddInvalidError(
			field.NewPath("spec").Child("zone"),
			rover.Spec.Zone,
			fmt.Sprintf("zone %q does not support consumer failover. Either disable it or select a different zone", rover.Spec.Zone))
		return nil, valErr.BuildError()
	}

	if err := r.validatePermissions(valErr, rover); err != nil {
		return nil, err
	}

	r.validateExternalIDs(valErr, rover, zone)
	r.validatePermissionEntries(valErr, rover)

	if err := r.validateEventSupport(ctx, valErr, rover, environment, zone); err != nil {
		return nil, err
	}

	if err := MustNotHaveDuplicates(valErr, rover.Spec.Subscriptions, rover.Spec.Exposures); err != nil {
		return nil, err
	}

	if err := r.validateSubscriptions(ctx, log, valErr, rover, environment); err != nil {
		return nil, err
	}

	if err := r.validateExposures(ctx, log, valErr, rover, environment, zoneRef); err != nil {
		return nil, err
	}

	return valErr.BuildWarnings(), valErr.BuildError()
}

func (r *RoverValidator) validateZone(ctx context.Context, valErr *cerrors.ValidationError, rover *roverv1.Rover, environment string) (client.ObjectKey, *adminv1.Zone, error) {
	zoneRef := client.ObjectKey{
		Name:      rover.Spec.Zone,
		Namespace: environment,
	}
	zone := &adminv1.Zone{}
	if exists, err := r.ResourceMustExist(ctx, zoneRef, zone); !exists {
		if err != nil {
			return zoneRef, nil, err
		}
		valErr.AddInvalidError(field.NewPath("spec").Child("zone"), rover.Spec.Zone, fmt.Sprintf("zone '%s' not found", rover.Spec.Zone))
		return zoneRef, nil, nil
	}

	return zoneRef, zone, nil
}

func (r *RoverValidator) validatePermissions(valErr *cerrors.ValidationError, rover *roverv1.Rover) error {
	if cconfig.FeaturePermission.IsEnabled() || len(rover.Spec.Permissions) == 0 {
		return nil
	}

	valErr.AddInvalidError(
		field.NewPath("spec").Child("zone"),
		rover.Spec.Zone,
		fmt.Sprintf("zone '%s' does not support permissions", rover.Spec.Zone),
	)
	return valErr.BuildError()
}

func (r *RoverValidator) validateExternalIDs(valErr *cerrors.ValidationError, rover *roverv1.Rover, zone *adminv1.Zone) {
	entries := make([]externalIdEntry, len(rover.Spec.ExternalIds))
	for i, eid := range rover.Spec.ExternalIds {
		entries[i] = externalIdEntry{Scheme: eid.Scheme, Id: eid.Id}
	}

	validateExternalIds(valErr, entries, zone, field.NewPath("spec").Child("externalIds"))
}

func (r *RoverValidator) validatePermissionEntries(valErr *cerrors.ValidationError, rover *roverv1.Rover) {
	// This validation is done here in the webhook rather than via CEL in the CRD because CEL rules with
	// .all() iteration over the entries array would exceed the Kubernetes validation cost budget by
	// over 40x (even with MaxItems=50). Webhook validation has no such budget constraints.
	//
	// The validation ensures that permission entries will normalize into valid PermissionSet specs where
	// both role and resource are required fields:
	// - Resource-oriented format (resource + entries): each entry must have a non-empty role
	// - Role-oriented format (role + entries): each entry must have a non-empty resource
	// - Flat format (role + resource + actions): validated by existing CEL rules, no nested entries
	for i, perm := range rover.Spec.Permissions {
		permPath := field.NewPath("spec").Child("permissions").Index(i)

		if perm.Resource != "" && len(perm.Entries) > 0 {
			for j, entry := range perm.Entries {
				if entry.Role == "" {
					valErr.AddInvalidError(
						permPath.Child("entries").Index(j).Child("role"),
						"",
						"role is required when parent permission has resource set (resource-oriented format)",
					)
				}
			}
		}

		if perm.Role != "" && len(perm.Entries) > 0 {
			for j, entry := range perm.Entries {
				if entry.Resource == "" {
					valErr.AddInvalidError(
						permPath.Child("entries").Index(j).Child("resource"),
						"",
						"resource is required when parent permission has role set (role-oriented format)",
					)
				}
			}
		}
	}
}

func (r *RoverValidator) validateEventSupport(ctx context.Context, valErr *cerrors.ValidationError, rover *roverv1.Rover, environment string, zone *adminv1.Zone) error {
	subscribesToEvents := slices.ContainsFunc(rover.Spec.Subscriptions, func(sub roverv1.Subscription) bool {
		return sub.Type() == roverv1.TypeEvent
	})
	exposesEvents := slices.ContainsFunc(rover.Spec.Exposures, func(exp roverv1.Exposure) bool {
		return exp.Type() == roverv1.TypeEvent
	})
	if !cconfig.FeaturePubSub.IsEnabled() || (!subscribesToEvents && !exposesEvents) {
		return nil
	}

	eventConfigRef := client.ObjectKey{
		Name:      environment,
		Namespace: zone.Status.Namespace,
	}
	eventConfig := eventv1.EventConfig{}
	if exists, err := r.ResourceMustExist(ctx, eventConfigRef, &eventConfig); !exists {
		if err != nil {
			return err
		}
		valErr.AddInvalidError(field.NewPath("spec").Child("zone"), rover.Spec.Zone, fmt.Sprintf("zone '%s' does not support event subscriptions or exposures", rover.Spec.Zone))
	}

	return nil
}

func (r *RoverValidator) validateSubscriptions(ctx context.Context, log logr.Logger, valErr *cerrors.ValidationError, rover *roverv1.Rover, environment string) error {
	for i, sub := range rover.Spec.Subscriptions {
		log.V(2).Info("validate subscription", "index", i, "subscription", sub)
		if err := r.ValidateSubscription(ctx, valErr, environment, sub, i); err != nil {
			return err
		}
	}

	return nil
}

func (r *RoverValidator) validateExposures(ctx context.Context, log logr.Logger, valErr *cerrors.ValidationError, rover *roverv1.Rover, environment string, zoneRef client.ObjectKey) error {
	for i, exposure := range rover.Spec.Exposures {
		log.V(2).Info("validate exposure", "index", i, "exposure", exposure)
		if err := r.ValidateExposure(ctx, valErr, environment, exposure, zoneRef, i); err != nil {
			return err
		}
	}

	return nil
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
	case roverv1.TypeAi:
		return r.ValidateAiExposure(ctx, valErr, environment, exposure, zoneRef, idx)
	default:
		valErr.AddInvalidError(
			field.NewPath("spec").Child("exposures").Index(idx).Child("type"),
			exposure.Type(), fmt.Sprintf("unknown exposure type %q", exposure.Type()),
		)
		return nil
	}
}

func (r *RoverValidator) validateExposureRateLimit(valErr *cerrors.ValidationError, exposure roverv1.Exposure, idx int) {
	// Check if API is nil
	if exposure.Api == nil {
		return
	}

	// Check if Traffic is nil
	if exposure.Api.Traffic == nil {
		return
	}

	// Check if RateLimit is nil
	if exposure.Api.Traffic.RateLimit == nil {
		return
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
		return
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

	checkDuplicate := func(existing map[string]bool, value string, path *field.Path, message string) {
		if _, exists := existing[value]; exists {
			valErr.AddInvalidError(path, value, message)
		}
		existing[value] = true
	}

	existingSubs := make(map[string]bool)
	for idx, sub := range subs {
		if sub.Api != nil {
			checkDuplicate(
				existingSubs,
				sub.Api.BasePath,
				field.NewPath("spec").Child("subscriptions").Index(idx).Child("api").Child("basePath"),
				fmt.Sprintf("duplicate subscription for base path %s", sub.Api.BasePath),
			)
		}

		if sub.Event != nil {
			checkDuplicate(
				existingSubs,
				sub.Event.EventType,
				field.NewPath("spec").Child("subscriptions").Index(idx).Child("event").Child("eventType"),
				fmt.Sprintf("duplicate subscription for event-type %s", sub.Event.EventType),
			)
		}

		if sub.Ai != nil {
			checkDuplicate(
				existingSubs,
				sub.Ai.BasePath,
				field.NewPath("spec").Child("subscriptions").Index(idx).Child("ai").Child("basePath"),
				fmt.Sprintf("duplicate subscription for ai base path %s", sub.Ai.BasePath),
			)
		}
	}

	existingExps := make(map[string]bool)
	for idx, exposure := range exps {
		if exposure.Api != nil {
			checkDuplicate(
				existingExps,
				exposure.Api.BasePath,
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("basePath"),
				fmt.Sprintf("duplicate exposure for base path %s", exposure.Api.BasePath),
			)
		}

		if exposure.Event != nil {
			checkDuplicate(
				existingExps,
				exposure.Event.EventType,
				field.NewPath("spec").Child("exposures").Index(idx).Child("event").Child("eventType"),
				fmt.Sprintf("duplicate exposure for event-type %s", exposure.Event.EventType),
			)
		}

		if exposure.Ai != nil {
			checkDuplicate(
				existingExps,
				exposure.Ai.BasePath,
				field.NewPath("spec").Child("exposures").Index(idx).Child("ai").Child("basePath"),
				fmt.Sprintf("duplicate exposure for ai base path %s", exposure.Ai.BasePath),
			)
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
	zone, err := r.GetZone(ctx, valErr, zoneRef)
	if err != nil {
		return err
	}
	if zone == nil {
		// Zone-missing error has been recorded on valErr; nothing further to check.
		return nil
	}
	if exp.Api.Transformation == nil || len(exp.Api.Transformation.Request.Headers.Remove) == 0 {
		return nil
	}

	for _, header := range exp.Api.Transformation.Request.Headers.Remove {
		if strings.EqualFold(header, "Authorization") && zone.Spec.Visibility != adminv1.ZoneVisibilityWorld {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("transformation").Child("request").Child("headers").Child("remove"),
				header, "removing 'Authorization' header is only allowed on external zones",
			)
		}
	}

	return nil
}

// GetZone fetches the Zone referenced by zoneRef. A not-found Zone is recorded
// on valErr against spec.zone so it joins any other accumulated validation
// errors; the returned (*adminv1.Zone) is nil in that case and the returned
// error is nil. A returned error means a real API failure (not a validation
// failure).
func (r *RoverValidator) GetZone(ctx context.Context, valErr *cerrors.ValidationError, zoneRef client.ObjectKey) (*adminv1.Zone, error) {
	zone := &adminv1.Zone{}
	err := r.client.Get(ctx, zoneRef, zone)
	if err != nil {
		if apierrors.IsNotFound(err) {
			valErr.AddInvalidError(
				field.NewPath("spec").Child("zone"),
				zoneRef.Name,
				fmt.Sprintf("zone %q not found", zoneRef.Name),
			)
			return nil, nil
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

// isNonRoutableTarget reports whether the given URL points at an address that is
// not reachable from outside the cluster and must therefore never be used as
// an upstream or callback target. This blocks:
//   - localhost and cluster-internal DNS names (*.svc, *.cluster.local, and
//     bare dotless names like "kubernetes" that resolve via search domains)
//   - loopback IPs (127.0.0.0/8, ::1) and the unspecified address (0.0.0.0, ::)
//   - link-local addresses (169.254.0.0/16, fe80::/10) — notably the cloud
//     metadata endpoint 169.254.169.254, which can leak node IAM credentials
//
// Private ranges (10/8, 172.16/12, 192.168/16) are deliberately NOT blocked:
// legitimate corporate/on-prem backends may live there, so blocking them is a
// deployment-specific policy decision rather than a universal safety rule.
//
// This is a best-effort, syntactic check only. It does NOT resolve hostnames,
// so DNS names that resolve to blocked IPs (e.g. 127.0.0.1.nip.io) are not
// caught here — that is left to runtime egress controls. The goal is to catch
// obvious misconfigurations at admission time, not to be a complete SSRF guard.
func isNonRoutableTarget(rawURL string) bool {
	if rawURL == "" {
		// Empty is handled by other validation (e.g. required-field / CRD rules).
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		// Unparseable URLs fail other validation; don't flag them here.
		return false
	}
	// Reject userinfo (user@host): it invites parser-differential confusion
	// across downstream components (e.g. "http://public.example.com@127.0.0.1/").
	if u.User != nil {
		return true
	}
	// Strip a trailing dot so the FQDN form "localhost." is treated as "localhost".
	host := strings.TrimSuffix(u.Hostname(), ".")

	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() ||
			ip.IsUnspecified() ||
			ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast()
	}

	// Not a literal IP: block localhost and cluster-internal DNS names.
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	if strings.HasSuffix(lower, ".cluster.local") {
		return true
	}
	// Kubernetes service DNS: my-svc.my-ns.svc[.cluster.local]. The ".svc"
	// suffix covers the common in-cluster form even without ".cluster.local".
	if strings.HasSuffix(lower, ".svc") || strings.Contains(lower, ".svc.") {
		return true
	}
	// Bare, dotless hostnames resolve against the cluster's search domains
	// (e.g. "kubernetes" -> kubernetes.default.svc.cluster.local).
	if !strings.Contains(lower, ".") {
		return true
	}
	return false
}

// validateExternalURL runs the full validation policy for any user-supplied
// URL: it must be http(s) and must not point at a cluster-internal or local
// address. Errors are added under the given field path.
func validateExternalURL(valErr *cerrors.ValidationError, path *field.Path, rawURL string) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		valErr.AddInvalidError(path, rawURL, "URL must start with http:// or https://")
	}
	if isNonRoutableTarget(rawURL) {
		valErr.AddInvalidError(path, rawURL,
			"URL must not point at a cluster-internal or local address (e.g. localhost, a loopback/link-local IP, or a *.cluster.local name)")
	}
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
		validateExternalURL(valErr,
			field.NewPath("spec").Child("exposures").Index(idx).Child("api").Child("upstreams").Index(i).Child("url"),
			upstream.URL)
	}

	// Validate rate limits if they are set
	r.validateExposureRateLimit(valErr, exposure, idx)

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

func (r *RoverValidator) ValidateAiExposure(ctx context.Context, valErr *cerrors.ValidationError, environment string, exposure roverv1.Exposure, zoneRef client.ObjectKey, idx int) error {
	if exposure.Ai == nil {
		return nil
	}
	for i, upstream := range exposure.Ai.Upstreams {
		if upstream.URL == "" {
			valErr.AddRequiredError(
				field.NewPath("spec").Child("exposures").Index(idx).Child("ai").Child("upstreams").Index(i).Child("url"),
				"upstream URL must not be empty",
			)
			continue
		}
		validateExternalURL(valErr,
			field.NewPath("spec").Child("exposures").Index(idx).Child("ai").Child("upstreams").Index(i).Child("url"),
			upstream.URL)
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
		if sub.Event.Delivery.Callback != "" {
			validateExternalURL(valErr,
				field.NewPath("spec").Child("subscriptions").Index(idx).Child("event").Child("delivery").Child("callback"),
				sub.Event.Delivery.Callback)
		}
		return nil
	case roverv1.TypeAi:
		return nil // AI subscriptions have no special validation at this time
	}

	return nil
}
