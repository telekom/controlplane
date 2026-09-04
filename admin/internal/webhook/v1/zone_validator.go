// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
)

// +kubebuilder:webhook:path=/validate-admin-cp-ei-telekom-de-v1-zone,mutating=false,failurePolicy=fail,sideEffects=None,groups=admin.cp.ei.telekom.de,resources=zones,verbs=create;update,versions=v1,name=vzone-v1.kb.io,admissionReviewVersions=v1

type ZoneCustomValidator struct{}

var _ admission.Validator[*adminv1.Zone] = &ZoneCustomValidator{}

func (v *ZoneCustomValidator) ValidateCreate(_ context.Context, zone *adminv1.Zone) (admission.Warnings, error) {
	return failoverWarnings(zone), validateZone(zone)
}

func (v *ZoneCustomValidator) ValidateUpdate(_ context.Context, oldZone, newZone *adminv1.Zone) (admission.Warnings, error) {
	errs := validateRetainedNames(oldZone, newZone)
	errs = append(errs, validateZoneFields(newZone)...)
	return failoverWarnings(newZone), invalidZone(newZone.Name, errs)
}

func (v *ZoneCustomValidator) ValidateDelete(_ context.Context, _ *adminv1.Zone) (admission.Warnings, error) {
	return nil, nil
}

// failoverWarnings flags ConsumerFailover on a non-API preset. The model permits it on
// every traffic type, but only the API domain implements failover routing today.
func failoverWarnings(zone *adminv1.Zone) admission.Warnings {
	var warnings admission.Warnings
	for i := range zone.Spec.Presets {
		preset := &zone.Spec.Presets[i]
		if preset.Type == adminv1.GatewayTypeAPI {
			continue
		}
		if preset.SupportsFeatures([]adminv1.FeatureName{adminv1.FeatureConsumerFailover}) {
			warnings = append(warnings, "preset "+strconv.Quote(preset.Name)+" enables ConsumerFailover for gateway type "+
				strconv.Quote(string(preset.Type))+", but failover routing is currently implemented for API traffic only")
		}
	}
	return warnings
}

func validateZone(zone *adminv1.Zone) error {
	return invalidZone(zone.Name, validateZoneFields(zone))
}

func invalidZone(name string, errs field.ErrorList) error {
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(schema.GroupKind{Group: adminv1.GroupVersion.Group, Kind: "Zone"}, name, errs)
}

func validateZoneFields(zone *adminv1.Zone) field.ErrorList { //nolint:gocyclo // Validation intentionally reports all independent field errors at once.
	specPath := field.NewPath("spec")
	var errs field.ErrorList

	errs = append(errs, validateUniqueNames(specPath.Child("gateways"), "gateway", func(yield func(string, int)) {
		for i, gateway := range zone.Spec.Gateways {
			yield(gateway.Name, i)
		}
	})...)
	errs = append(errs, validateUniqueNames(specPath.Child("identityProviders"), "identity provider", func(yield func(string, int)) {
		for i, identityProvider := range zone.Spec.IdentityProviders {
			yield(identityProvider.Name, i)
		}
	})...)

	if len(zone.Spec.IdentityProviders) != 1 {
		errs = append(errs, field.Invalid(specPath.Child("identityProviders"), len(zone.Spec.IdentityProviders), "exactly one identity provider is required"))
	}
	soleIdentityProviderName := ""
	identityProviderTokenURL := ""
	if identityProvider, err := zone.Spec.GetIdentityProvider(); err == nil {
		soleIdentityProviderName = identityProvider.Name
		identityProviderTokenURL = identityProvider.TokenUrl
	}

	failoverTokenURL := ""
	errs = append(errs, validateFeatures(specPath.Child("features"), zone.Spec.Features, adminv1.FeatureScopeZone)...)
	for i := range zone.Spec.Presets {
		preset := &zone.Spec.Presets[i]
		presetPath := specPath.Child("presets").Index(i)
		if _, err := zone.Spec.GetGateway(preset.GatewayRef); err != nil {
			errs = append(errs, field.Invalid(presetPath.Child("gatewayRef"), preset.GatewayRef, err.Error()))
		}
		if _, err := zone.Spec.GetIdentityProviderByName(preset.IdentityProviderRef); err != nil {
			errs = append(errs, field.Invalid(presetPath.Child("identityProviderRef"), preset.IdentityProviderRef, err.Error()))
		} else if preset.IdentityProviderRef != soleIdentityProviderName {
			errs = append(errs, field.Invalid(presetPath.Child("identityProviderRef"), preset.IdentityProviderRef, "must reference the sole identity provider "+strconv.Quote(soleIdentityProviderName)))
		}
		if preset.TokenUrl != "" {
			errs = append(errs, validateTokenURL(presetPath.Child("tokenUrl"), preset.TokenUrl)...)
		}
		// API and AI failover presets share the zone's identity client and therefore one token URL.
		if preset.SupportsFeatures([]adminv1.FeatureName{adminv1.FeatureConsumerFailover}) {
			effectiveTokenURL := preset.TokenUrl
			if effectiveTokenURL == "" {
				effectiveTokenURL = identityProviderTokenURL
			}
			if effectiveTokenURL != "" && failoverTokenURL != "" && effectiveTokenURL != failoverTokenURL {
				errs = append(errs, field.Invalid(presetPath.Child("tokenUrl"), preset.TokenUrl, "all ConsumerFailover presets must use the same token URL"))
			} else if effectiveTokenURL != "" {
				failoverTokenURL = effectiveTokenURL
			}
		}
		if preset.GetDefaultURL() == "" {
			errs = append(errs, field.Invalid(presetPath.Child("urls"), preset.Urls, "at least one non-hidden URL is required"))
		}
		for j := range preset.Urls {
			urlPath := presetPath.Child("urls").Index(j)
			errs = append(errs, validateHostname(urlPath.Child("hostname"), preset.Urls[j].Hostname)...)
			errs = append(errs, validateBasePath(urlPath.Child("basePath"), preset.Urls[j].BasePath)...)
		}
		errs = append(errs, validateFeatures(presetPath.Child("features"), preset.Features, adminv1.FeatureScopePreset)...)
	}
	for i := range zone.Spec.Gateways {
		gateway := &zone.Spec.Gateways[i]
		gatewayPath := specPath.Child("gateways").Index(i)
		if _, err := zone.Spec.GetIdentityProviderByName(gateway.Admin.IdentityProviderRef); err != nil {
			errs = append(errs, field.Invalid(gatewayPath.Child("admin", "identityProviderRef"), gateway.Admin.IdentityProviderRef, err.Error()))
		} else if gateway.Admin.IdentityProviderRef != soleIdentityProviderName {
			errs = append(errs, field.Invalid(gatewayPath.Child("admin", "identityProviderRef"), gateway.Admin.IdentityProviderRef, "must reference the sole identity provider "+strconv.Quote(soleIdentityProviderName)))
		}
		errs = append(errs, validateHTTPSURL(gatewayPath.Child("admin", "url"), gateway.Admin.Url)...)
	}

	for i := range zone.Spec.IdentityProviders {
		identityProvider := &zone.Spec.IdentityProviders[i]
		idpPath := specPath.Child("identityProviders").Index(i)
		if identityProvider.Admin.Url != nil {
			errs = append(errs, validateHTTPSURL(idpPath.Child("admin", "url"), *identityProvider.Admin.Url)...)
		}
		if identityProvider.TokenUrl != "" {
			errs = append(errs, validateTokenURL(idpPath.Child("tokenUrl"), identityProvider.TokenUrl)...)
		}
		if identityProvider.IssuerHostname != "" {
			errs = append(errs, validateHostname(idpPath.Child("issuerHostname"), identityProvider.IssuerHostname)...)
		} else if identityProvider.Admin.Url == nil || !hasUsableURLHostname(*identityProvider.Admin.Url) {
			errs = append(errs, field.Required(idpPath.Child("issuerHostname"), "must be set when admin.url has no usable hostname"))
		}
	}

	errs = append(errs, validatePresetTypes(zone)...)
	errs = append(errs, validateGatewayReferences(zone)...)

	return errs
}

// enabledFeature is a preset-scoped feature that a preset turns on, carrying its index in
// the preset's feature list so an error can point at the entry rather than the whole list.
type enabledFeature struct {
	name  adminv1.FeatureName
	index int
}

// validatePresetTypes enforces the per-type preset rules that make selection single-valued:
// one feature-free default per type, every other preset carrying at least one enabled
// preset-scoped feature, and no enabled feature repeated within a type.
func validatePresetTypes(zone *adminv1.Zone) field.ErrorList {
	presetsPath := field.NewPath("spec", "presets")
	var errs field.ErrorList

	defaults := make(map[adminv1.GatewayType]int)
	featureOwners := make(map[adminv1.GatewayType]map[adminv1.FeatureName]string)
	types := make([]adminv1.GatewayType, 0, len(zone.Spec.Presets))

	for i := range zone.Spec.Presets {
		preset := &zone.Spec.Presets[i]
		presetPath := presetsPath.Index(i)
		if preset.Type == "" {
			// The API server rejects this; skip so the remaining rules stay meaningful.
			continue
		}
		if !slices.Contains(types, preset.Type) {
			types = append(types, preset.Type)
		}

		var enabled []enabledFeature
		for j, feature := range preset.Features {
			if feature.Enabled && adminv1.FeatureScopeOf(feature.Name) == adminv1.FeatureScopePreset {
				enabled = append(enabled, enabledFeature{name: feature.Name, index: j})
			}
		}

		if preset.Default {
			defaults[preset.Type]++
			if len(enabled) > 0 {
				errs = append(errs, field.Invalid(presetPath.Child("features"), preset.Features,
					"default preset must not enable preset-scoped features, otherwise normal traffic uses a feature profile"))
			}
			continue
		}

		if len(enabled) == 0 {
			errs = append(errs, field.Invalid(presetPath.Child("features"), preset.Features,
				"a non-default preset must enable at least one preset-scoped feature, otherwise it can never be selected"))
		}
		for _, feature := range enabled {
			owners, ok := featureOwners[preset.Type]
			if !ok {
				owners = make(map[adminv1.FeatureName]string)
				featureOwners[preset.Type] = owners
			}
			if owner, found := owners[feature.name]; found {
				errs = append(errs, field.Duplicate(presetPath.Child("features").Index(feature.index).Child("name"),
					"feature "+strconv.Quote(string(feature.name))+" is already enabled on preset "+strconv.Quote(owner)+
						" for gateway type "+strconv.Quote(string(preset.Type))))
				continue
			}
			owners[feature.name] = preset.Name
		}
	}

	for _, gatewayType := range types {
		if defaults[gatewayType] != 1 {
			errs = append(errs, field.Invalid(presetsPath, defaults[gatewayType],
				"exactly one default preset is required for gateway type "+strconv.Quote(string(gatewayType))))
		}
	}

	// The API type carries the zone's representative profile: GetDefaultPreset, and every
	// caller that has no traffic type of its own, resolves through it. Without one the zone
	// cannot be reconciled at all, so reject it here instead of at reconcile time.
	if !slices.Contains(types, adminv1.GatewayTypeAPI) {
		errs = append(errs, field.Required(presetsPath,
			"at least one preset of gateway type "+strconv.Quote(string(adminv1.GatewayTypeAPI))+
				" is required: it is the zone's representative profile for callers that carry no gateway type"))
	}

	return errs
}

// validateGatewayReferences rejects a gateway no preset uses. Preset references are the
// only reachability signal now, and an unused gateway still provisions a Gateway, an admin
// client and a consumer.
func validateGatewayReferences(zone *adminv1.Zone) field.ErrorList {
	var errs field.ErrorList
	for i := range zone.Spec.Gateways {
		gateway := &zone.Spec.Gateways[i]
		used := slices.ContainsFunc(zone.Spec.Presets, func(preset adminv1.Preset) bool {
			return preset.GatewayRef == gateway.Name
		})
		if !used {
			errs = append(errs, field.Invalid(field.NewPath("spec", "gateways").Index(i).Child("name"), gateway.Name,
				"gateway is not referenced by any preset and would serve no traffic"))
		}
	}
	return errs
}

func validateFeatures(path *field.Path, features []adminv1.Feature, expected adminv1.FeatureScope) field.ErrorList {
	seen := make(map[adminv1.FeatureName]struct{}, len(features))
	var errs field.ErrorList
	for i, feature := range features {
		featurePath := path.Index(i).Child("name")
		if _, found := seen[feature.Name]; found {
			errs = append(errs, field.Duplicate(featurePath, "duplicate feature "+strconv.Quote(string(feature.Name))))
		}
		seen[feature.Name] = struct{}{}
		scope := adminv1.FeatureScopeOf(feature.Name)
		switch {
		case scope == adminv1.FeatureScopeUnknown:
			errs = append(errs, field.Invalid(featurePath, feature.Name, "unknown feature"))
		case scope != expected:
			errs = append(errs, field.Invalid(featurePath, feature.Name, strings.ToLower(string(scope))+"-scoped feature cannot be configured at "+strings.ToLower(string(expected))+" scope"))
		}
	}
	return errs
}

func validateRetainedNames(oldZone, newZone *adminv1.Zone) field.ErrorList {
	var errs field.ErrorList
	for _, gateway := range oldZone.Spec.Gateways {
		if _, err := newZone.Spec.GetGateway(gateway.Name); err != nil {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "gateways"), "existing gateway name "+strconv.Quote(gateway.Name)+" cannot be removed or renamed"))
		}
	}
	for _, identityProvider := range oldZone.Spec.IdentityProviders {
		if _, err := newZone.Spec.GetIdentityProviderByName(identityProvider.Name); err != nil {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "identityProviders"), "existing identity provider name "+strconv.Quote(identityProvider.Name)+" cannot be removed or renamed"))
		}
	}
	return errs
}

func validateUniqueNames(path *field.Path, kind string, names func(func(string, int))) field.ErrorList {
	seen := make(map[string]struct{})
	var errs field.ErrorList
	names(func(name string, index int) {
		if _, found := seen[name]; found {
			errs = append(errs, field.Duplicate(path.Index(index).Child("name"), "duplicate "+kind+" name "+strconv.Quote(name)))
		}
		seen[name] = struct{}{}
	})
	return errs
}

func hasUsableURLHostname(rawURL string) bool {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil || !u.IsAbs() || u.Hostname() == "" {
		return false
	}
	return len(validateHostname(field.NewPath("hostname"), u.Hostname())) == 0
}

func validateHTTPSURL(path *field.Path, rawURL string) field.ErrorList {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil || !u.IsAbs() || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.Contains(rawURL, "#") || containsControl(rawURL) {
		return field.ErrorList{field.Invalid(path, rawURL, "must be an absolute HTTPS URL with a hostname and without userinfo or fragment")}
	}
	if strings.Contains(u.Host, ":") {
		if _, _, err := net.SplitHostPort(u.Host); err != nil {
			return field.ErrorList{field.Invalid(path, rawURL, "port must be between 1 and 65535")}
		}
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return field.ErrorList{field.Invalid(path, rawURL, "port must be between 1 and 65535")}
		}
	}
	return nil
}

func validateTokenURL(path *field.Path, rawURL string) field.ErrorList {
	if errs := validateHTTPSURL(path, rawURL); len(errs) != 0 {
		return errs
	}
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return field.ErrorList{field.Invalid(path, rawURL, "must be a valid URL")}
	}
	if u.RawQuery != "" {
		return field.ErrorList{field.Invalid(path, rawURL, "must not contain query parameters")}
	}
	return nil
}

func validateHostname(path *field.Path, hostname string) field.ErrorList {
	if hostname == "" || containsControl(hostname) {
		return field.ErrorList{field.Invalid(path, hostname, "must be a hostname without URL components")}
	}
	if net.ParseIP(hostname) != nil {
		return nil
	}
	if strings.ContainsAny(hostname, ":/?#@*") || len(utilvalidation.IsDNS1123Subdomain(hostname)) != 0 {
		return field.ErrorList{field.Invalid(path, hostname, "must be a valid hostname without scheme, port, path, wildcard, userinfo, query, or fragment")}
	}
	return nil
}

func validateBasePath(path *field.Path, basePath string) field.ErrorList {
	if !strings.HasPrefix(basePath, "/") || strings.ContainsAny(basePath, "?#") || containsControl(basePath) {
		return field.ErrorList{field.Invalid(path, basePath, "must start with / and contain no query or fragment")}
	}
	if _, err := url.ParseRequestURI(basePath); err != nil {
		return field.ErrorList{field.Invalid(path, basePath, "must be a valid URL path")}
	}
	return nil
}

func containsControl(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}
