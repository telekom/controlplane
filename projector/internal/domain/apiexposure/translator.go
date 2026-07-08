// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"strings"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"github.com/telekom/controlplane/projector/internal/util"
	"k8s.io/apimachinery/pkg/types"
)

// applicationLabelKey is the label key used by the Rover controller to
// associate an ApiExposure CR with its owner Application.
const applicationLabelKey = "cp.ei.telekom.de/application"

// Translator maps an ApiExposure CR to an APIExposureData DTO and derives
// identity keys.
//
// ApiExposure uses a convention-based fallback delete strategy: when lastKnown
// is available, KeyFromDelete reads Spec.ApiBasePath + label for app name +
// namespace for team name. Otherwise, it falls back to using key.Name for
// both basePath and appName + TeamNameFromNamespace. This always succeeds —
// no ErrDeleteKeyLost.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*apiv1.ApiExposure, *APIExposureData, APIExposureKey] = (*Translator)(nil)

// ShouldSkip returns false — ApiExposure CRs are always syncable.
func (t *Translator) ShouldSkip(_ *apiv1.ApiExposure) (bool, string) {
	return false, ""
}

// Translate converts an ApiExposure CR into an APIExposureData DTO.
// Visibility is upper-cased (World→WORLD, Zone→ZONE, Enterprise→ENTERPRISE).
// Active comes from Status.Active. Features defaults to an empty slice.
// Upstreams are mapped 1:1. ApprovalConfig strategy is mapped from PascalCase
// to DB enum (Auto→AUTO, Simple→SIMPLE, FourEyes→FOUR_EYES). APIVersion is
// always nil. AppName comes from the application label, TeamName from namespace.
func (t *Translator) Translate(_ context.Context, obj *apiv1.ApiExposure) (*APIExposureData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	upstreams := make([]model.Upstream, len(obj.Spec.Upstreams))
	for i, u := range obj.Spec.Upstreams {
		upstreams[i] = model.Upstream{
			URL:    u.Url,
			Weight: u.Weight,
		}
	}

	var security *model.ApiExposureSecurity
	if obj.Spec.Security != nil {
		security = &model.ApiExposureSecurity{}
		if obj.Spec.Security.M2M != nil {
			security.M2M = &model.Machine2MachineAuthentication{}
			if obj.Spec.Security.M2M.Basic != nil {
				security.M2M.Basic = util.MapCrBasicAuthToCpApi(obj.Spec.Security.M2M.Basic)
			}
			if obj.Spec.Security.M2M.ExternalIDP != nil {
				security.M2M.ExternalIDP = util.MapCrExternalIdpToCpApi(obj.Spec.Security.M2M.ExternalIDP)
			}
			if len(obj.Spec.Security.M2M.Scopes) > 0 {
				security.M2M.Scopes = obj.Spec.Security.M2M.Scopes
			}
		}
	}

	traffic := &model.Traffic{}
	if obj.Spec.Traffic.RateLimit != nil {
		traffic.RateLimit = &model.RateLimit{}
		if obj.Spec.Traffic.RateLimit.Provider != nil {
			traffic.RateLimit.Provider = &model.RateLimitConfig{
				Limits:  model.Limits(obj.Spec.Traffic.RateLimit.Provider.Limits),
				Options: model.RateLimitOptions(obj.Spec.Traffic.RateLimit.Provider.Options),
			}
		}
		if obj.Spec.Traffic.RateLimit.SubscriberRateLimit != nil {
			traffic.RateLimit.SubscriberRateLimit = &model.SubscriberRateLimits{
				Overrides: []model.RateLimitOverrides{},
			}
			if obj.Spec.Traffic.RateLimit.SubscriberRateLimit.Default != nil {
				traffic.RateLimit.SubscriberRateLimit.Default = &model.SubscriberRateLimitDefaults{
					Limits: model.Limits(obj.Spec.Traffic.RateLimit.SubscriberRateLimit.Default.Limits),
				}
			}
			for i := range obj.Spec.Traffic.RateLimit.SubscriberRateLimit.Overrides {
				traffic.RateLimit.SubscriberRateLimit.Overrides = append(traffic.RateLimit.SubscriberRateLimit.Overrides,
					model.RateLimitOverrides{
						Subscriber: obj.Spec.Traffic.RateLimit.SubscriberRateLimit.Overrides[i].Subscriber,
						Limits:     model.Limits(obj.Spec.Traffic.RateLimit.SubscriberRateLimit.Overrides[i].Limits),
					},
				)
			}
		}
	}
	if obj.Spec.Traffic.Failover != nil {
		traffic.Failover = &model.Failover{}
		for i := range obj.Spec.Traffic.Failover.Zones {
			traffic.Failover.Zones = append(traffic.Failover.Zones, obj.Spec.Traffic.Failover.Zones[i].Name)
		}
	}

	features := mapFeatures(obj)

	return &APIExposureData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		BasePath:      obj.Spec.ApiBasePath,
		Visibility:    strings.ToUpper(string(obj.Spec.Visibility)),
		Active:        obj.Status.Active,
		Features:      features,
		Upstreams:     upstreams,
		ApprovalConfig: model.ApprovalConfig{
			Strategy:     mapApprovalStrategy(string(obj.Spec.Approval.Strategy)),
			TrustedTeams: obj.Spec.Approval.TrustedTeams,
		},
		APIVersion: nil,
		AppName:    obj.Labels[applicationLabelKey],
		TeamName:   shared.TeamNameFromNamespace(obj.Namespace),
		Security:   security,
		Traffic:    traffic,
	}, nil
}

// determineFeatures checks for the feature of a given APIExposures
func mapFeatures(obj *apiv1.ApiExposure) []string {
	featureList := []string{""}

	// SECURITY RELATED
	featureList = append(featureList, "LAST_MILE_SECURITY")
	if obj.Spec.Security != nil {
		if obj.Spec.Security.M2M != nil {
			if obj.Spec.Security.M2M.ExternalIDP != nil {
				featureList = append(featureList, "EXTERNAL_IDP")
			}
			if len(obj.Spec.Security.M2M.Scopes) > 0 {
				featureList = append(featureList, "CUSTOM_SCOPES")
			}
			if obj.Spec.Security.M2M.Basic != nil {
				featureList = append(featureList, "BASIC_AUTH")
			}
		}
	}

	// TRAFFIC RELATED
	if obj.Spec.Traffic.RateLimit != nil {
		featureList = append(featureList, "RATE_LIMIT")
	}
	if obj.Spec.Traffic.Failover != nil {
		featureList = append(featureList, "FAILOVER")
	}
	if obj.Spec.Traffic.CircuitBreaker != nil {
		featureList = append(featureList, "CIRCUIT_BREAKER")
	}

	if len(obj.Spec.Upstreams) > 1 {
		featureList = append(featureList, "LOAD_BALANCING")
	}

	// TRANSFORMATION
	if obj.Spec.Transformation != nil {
		featureList = append(featureList, "HEADER_TRANSFORMATION")
	}

	return featureList
}

// KeyFromObject derives the composite identity key from a live ApiExposure.
func (t *Translator) KeyFromObject(obj *apiv1.ApiExposure) APIExposureKey {
	return APIExposureKey{
		BasePath: obj.Spec.ApiBasePath,
		AppName:  obj.Labels[applicationLabelKey],
		TeamName: shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, basePath comes from Spec.ApiBasePath, appName
// from the application label, and teamName from the namespace. Otherwise,
// key.Name is used as best-effort for both basePath and appName, and teamName
// is derived from the namespace convention. Always succeeds.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *apiv1.ApiExposure) (APIExposureKey, error) {
	if lastKnown != nil {
		return APIExposureKey{
			BasePath: lastKnown.Spec.ApiBasePath,
			AppName:  lastKnown.Labels[applicationLabelKey],
			TeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return APIExposureKey{
		BasePath: req.Name,
		AppName:  req.Name,
		TeamName: shared.TeamNameFromNamespace(req.Namespace),
	}, nil
}

// mapApprovalStrategy converts CR approval strategy values to the DB enum
// representation. CR uses PascalCase (Auto, Simple, FourEyes), while the DB
// uses uppercase with underscores (AUTO, SIMPLE, FOUR_EYES).
func mapApprovalStrategy(strategy string) string {
	switch strategy {
	case "Auto":
		return "AUTO"
	case "Simple":
		return "SIMPLE"
	case "FourEyes":
		return "FOUR_EYES"
	default:
		return strings.ToUpper(strategy)
	}
}
