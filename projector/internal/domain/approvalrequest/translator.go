// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"strings"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an ApprovalRequest CR to an ApprovalRequestData DTO and
// derives identity keys.
//
// ShouldSkip filters out CRs that lack the required fields for FK resolution:
// empty spec.target.Name, empty spec.action, or spec.target.Kind != "ApiSubscription".
//
// KeyFromDelete uses lastKnown when available. When lastKnown is nil, returns
// a key with only Namespace+Name (sufficient for DB delete by unique index)
// but empty subscription fields (cache cleanup skipped).
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*approvalv1.ApprovalRequest, *ApprovalRequestData, ApprovalRequestKey] = (*Translator)(nil)

// ShouldSkip returns true if the ApprovalRequest CR lacks the required fields
// for sync (missing target name, empty action, or non-ApiSubscription target
// kind).
func (t *Translator) ShouldSkip(obj *approvalv1.ApprovalRequest) (bool, string) {
	if obj.Spec.Target.Name == "" {
		return true, "spec.target.name is empty"
	}
	if obj.Spec.Action == "" {
		return true, "spec.action is empty"
	}
	// TODO: This filter should be removed in the future when other target kinds are supported.
	if obj.Spec.Target.TypeMeta.Kind != "ApiSubscription" {
		return true, "spec.target.kind is not ApiSubscription"
	}

	if obj.Spec.Decider.TeamName == "" {
		return true, "spec.decider.teamName is empty"
	}

	return false, ""
}

// Translate converts an ApprovalRequest CR into an ApprovalRequestData DTO.
//
// Enum conversions: CR uses PascalCase (Auto, Pending, FourEyes, ...); the ent
// DB uses SCREAMING_SNAKE (AUTO, PENDING, FOUR_EYES, ...).
//
// AvailableTransitions are read from Status (not Spec) because they are
// computed by the approval-operator's FSM.
//
// The subscription reference is derived from spec.target, which carries the
// k8s namespace and name of the ApiSubscription CR being approved. If the
// target namespace is empty, it falls back to the ApprovalRequest CR's own
// namespace (same-namespace reference).
func (t *Translator) Translate(_ context.Context, obj *approvalv1.ApprovalRequest) (*ApprovalRequestData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	targetNamespace := obj.Spec.Target.Namespace
	if targetNamespace == "" {
		targetNamespace = obj.Namespace
	}

	return &ApprovalRequestData{
		Meta:                  shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:           phase,
		StatusMessage:         message,
		State:                 mapState(string(obj.Spec.State)),
		Action:                obj.Spec.Action,
		Strategy:              mapStrategy(string(obj.Spec.Strategy)),
		Requester:             mapRequester(obj.Spec.Requester),
		Decider:               mapDecider(obj.Spec.Decider),
		Decisions:             mapDecisions(obj.Spec.Decisions),
		AvailableTransitions:  mapAvailableTransitions(obj.Status.AvailableTransitions),
		SubscriptionNamespace: targetNamespace,
		SubscriptionName:      obj.Spec.Target.Name,
	}, nil
}

// KeyFromObject derives the composite identity key from a live
// ApprovalRequest.
func (t *Translator) KeyFromObject(obj *approvalv1.ApprovalRequest) ApprovalRequestKey {
	targetNamespace := obj.Spec.Target.Namespace
	if targetNamespace == "" {
		targetNamespace = obj.Namespace
	}
	return ApprovalRequestKey{
		Namespace:             obj.Namespace,
		Name:                  obj.Name,
		SubscriptionNamespace: targetNamespace,
		SubscriptionName:      obj.Spec.Target.Name,
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, all fields are taken from the spec + metadata.
// If lastKnown is nil, returns a key with only Namespace+Name (sufficient
// for DB delete by unique index) but empty subscription fields (cache cleanup
// skipped).
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *approvalv1.ApprovalRequest) (ApprovalRequestKey, error) {
	if lastKnown != nil {
		targetNamespace := lastKnown.Spec.Target.Namespace
		if targetNamespace == "" {
			targetNamespace = lastKnown.Namespace
		}
		return ApprovalRequestKey{
			Namespace:             lastKnown.Namespace,
			Name:                  lastKnown.Name,
			SubscriptionNamespace: targetNamespace,
			SubscriptionName:      lastKnown.Spec.Target.Name,
		}, nil
	}
	// Without lastKnown we can still delete by namespace+name (the DB unique
	// index), but we cannot clean the subscription-keyed cache entry.
	// We use empty subscription fields and let the repository handle it.
	return ApprovalRequestKey{
		Namespace:             req.Namespace,
		Name:                  req.Name,
		SubscriptionNamespace: "",
		SubscriptionName:      "",
	}, nil
}

// mapState converts a PascalCase CR state to the SCREAMING_SNAKE ent enum.
func mapState(state string) string {
	return strings.ToUpper(state)
}

// mapStrategy converts a PascalCase CR strategy to the SCREAMING_SNAKE ent
// enum, handling the special FourEyes -> FOUR_EYES case.
func mapStrategy(strategy string) string {
	switch strategy {
	case "FourEyes":
		return "FOUR_EYES"
	default:
		return strings.ToUpper(strategy)
	}
}

// mapRequester converts the CR Requester to the model RequesterInfo DTO.
// String fields are converted to *string where the model uses pointers.
func mapRequester(r approvalv1.Requester) model.RequesterInfo {
	info := model.RequesterInfo{
		TeamName:  r.TeamName,
		TeamEmail: r.TeamEmail,
	}
	if r.Reason != "" {
		info.Reason = &r.Reason
	}
	if r.ApplicationRef != nil && r.ApplicationRef.Name != "" {
		info.ApplicationName = &r.ApplicationRef.Name
	}
	return info
}

// mapDecider converts the CR Decider to the model DeciderInfo DTO.
func mapDecider(d approvalv1.Decider) model.DeciderInfo {
	info := model.DeciderInfo{
		TeamName: d.TeamName,
	}
	if d.TeamEmail != "" {
		info.TeamEmail = &d.TeamEmail
	}
	return info
}

// mapDecisions converts a slice of CR Decision to model Decision DTOs.
// String fields are converted to *string where the model uses pointers.
// Timestamp and ResultingState are left nil since they do not exist on the CR.
func mapDecisions(decisions []approvalv1.Decision) []model.Decision {
	if len(decisions) == 0 {
		return []model.Decision{}
	}
	result := make([]model.Decision, len(decisions))
	for i, d := range decisions {
		result[i] = model.Decision{
			Name: d.Name,
		}
		if d.Email != "" {
			result[i].Email = &d.Email
		}
		if d.Comment != "" {
			result[i].Comment = &d.Comment
		}
	}
	return result
}

// mapAvailableTransitions converts CR AvailableTransitions (from Status) to
// model AvailableTransition DTOs. Note: CR field .To maps to DTO .ToState.
func mapAvailableTransitions(transitions approvalv1.AvailableTransitions) []model.AvailableTransition {
	if len(transitions) == 0 {
		return []model.AvailableTransition{}
	}
	result := make([]model.AvailableTransition, len(transitions))
	for i, at := range transitions {
		result[i] = model.AvailableTransition{
			Action:  string(at.Action),
			ToState: string(at.To),
		}
	}
	return result
}
