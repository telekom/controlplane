// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import (
	"context"
	"strings"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"github.com/telekom/controlplane/projector/internal/util"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an AgenticExposure CR to an AgenticExposureData DTO and
// derives identity keys.
//
// Unlike ApiExposure (which derives AppName from a Rover-controller label),
// AgenticExposure derives AppName directly from Spec.Provider (an ObjectRef)
// — matching the ApiSubscription convention. This means KeyFromDelete can
// always resolve AppName from lastKnown without relying on labels.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*agenticv1.AgenticExposure, *AgenticExposureData, AgenticExposureKey] = (*Translator)(nil)

// ShouldSkip returns false — AgenticExposure CRs are always syncable.
func (t *Translator) ShouldSkip(_ *agenticv1.AgenticExposure) (bool, string) {
	return false, ""
}

// Translate converts an AgenticExposure CR into an AgenticExposureData DTO.
// Visibility is upper-cased (World→WORLD, Zone→ZONE, Enterprise→ENTERPRISE).
// Variant is used verbatim (already MCP/TELECONTEXTMCP/AGENT in the CR).
// Active comes from Status.Active. Upstreams are mapped 1:1. ApprovalConfig
// strategy is mapped from PascalCase to DB enum (Auto→AUTO, Simple→SIMPLE,
// FourEyes→FOUR_EYES). AppName comes from Spec.Provider, TeamName from namespace.
func (t *Translator) Translate(_ context.Context, obj *agenticv1.AgenticExposure) (*AgenticExposureData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	upstreams := make([]model.Upstream, len(obj.Spec.Upstreams))
	for i, u := range obj.Spec.Upstreams {
		upstreams[i] = model.Upstream{
			URL:    u.Url,
			Weight: u.Weight,
		}
	}

	var security *model.AgenticExposureSecurity
	if obj.Spec.Security != nil && obj.Spec.Security.M2M != nil {
		security = &model.AgenticExposureSecurity{}
		security.M2M = &model.Machine2MachineAuthentication{}
		if obj.Spec.Security.M2M.Basic != nil {
			security.M2M.Basic = util.MapAgenticBasicAuthToCpApi(obj.Spec.Security.M2M.Basic)
		}
		if obj.Spec.Security.M2M.ExternalIDP != nil {
			security.M2M.ExternalIDP = util.MapAgenticExternalIdpToCpApi(obj.Spec.Security.M2M.ExternalIDP)
		}
		if len(obj.Spec.Security.M2M.Scopes) > 0 {
			security.M2M.Scopes = obj.Spec.Security.M2M.Scopes
		}
	}

	var traffic *model.Traffic
	if obj.Spec.Traffic.RateLimit != nil || obj.Spec.Traffic.Failover != nil {
		traffic = &model.Traffic{}
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
	}

	var transformation *model.AgenticTransformation
	if obj.Spec.Transformation != nil {
		transformation = &model.AgenticTransformation{
			Request: model.AgenticRequestResponseTransformation{
				Headers: model.AgenticHeaderTransformation{
					Remove: obj.Spec.Transformation.Request.Headers.Remove,
					Add:    obj.Spec.Transformation.Request.Headers.Add,
				},
			},
		}
	}

	return &AgenticExposureData{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		BasePath:      obj.Spec.BasePath,
		Visibility:    strings.ToUpper(string(obj.Spec.Visibility)),
		Variant:       string(obj.Spec.Variant),
		Active:        obj.Status.Active,
		Upstreams:     upstreams,
		ApprovalConfig: model.ApprovalConfig{
			Strategy:     mapApprovalStrategy(string(obj.Spec.Approval.Strategy)),
			TrustedTeams: obj.Spec.Approval.TrustedTeams,
		},
		AppName:        obj.Spec.Provider.Name,
		TeamName:       shared.TeamNameFromNamespace(obj.Namespace),
		Security:       security,
		Traffic:        traffic,
		Transformation: transformation,
	}, nil
}

// KeyFromObject derives the composite identity key from a live AgenticExposure.
func (t *Translator) KeyFromObject(obj *agenticv1.AgenticExposure) AgenticExposureKey {
	return AgenticExposureKey{
		BasePath: obj.Spec.BasePath,
		AppName:  obj.Spec.Provider.Name,
		TeamName: shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, all fields are taken from the spec + namespace.
// Otherwise, key.Name is used as best-effort for both basePath and appName,
// and teamName is derived from the namespace convention. Always succeeds.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *agenticv1.AgenticExposure) (AgenticExposureKey, error) {
	if lastKnown != nil {
		return AgenticExposureKey{
			BasePath: lastKnown.Spec.BasePath,
			AppName:  lastKnown.Spec.Provider.Name,
			TeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return AgenticExposureKey{
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
