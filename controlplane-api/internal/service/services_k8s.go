// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// rotateKeyword is the keyword that triggers secret rotation via the webhook.
const rotateKeyword = "rotate"

// ----- Team -----

type teamK8sService struct {
	client cc.ScopedClient
}

// NewTeamK8sService creates a TeamService backed by Kubernetes.
func NewTeamK8sService(c cc.ScopedClient) TeamService {
	return &teamK8sService{client: c}
}

func (s *teamK8sService) CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.CreateTeamPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "CreateTeam", "group", input.Group, "team", input.Name)

	if err := authorizeCreateTeam(ctx, input.Group); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.CreateTeamPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	resourceName := organizationv1.TeamResourceName(input.Group, input.Name)
	namespace := input.Environment

	team := &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
	}

	_, err := s.client.CreateOrUpdate(ctx, team, func() error {
		team.Spec = organizationv1.TeamSpec{
			Name:     input.Name,
			Group:    input.Group,
			Email:    input.Email,
			Members:  toK8sMembers(input.Members),
			Category: organizationv1.TeamCategoryCustomer,
		}
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to create or update team resource", "resourceName", resourceName, "namespace", namespace)
		return &model.CreateTeamPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	log.V(0).Info("Created team", "resourceName", resourceName, "namespace", namespace)
	return &model.CreateTeamPayload{
		Accepted: true,
		Errors:   []model.MutationError{},
	}, nil
}

func (s *teamK8sService) UpdateTeam(ctx context.Context, ref ResourceRef, input model.UpdateTeamInput) (*model.UpdateTeamPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "UpdateTeam", "resourceName", ref.Name, "namespace", ref.Namespace)

	if err := authorizeUpdateTeam(ctx, ref.Group, ref.TeamName); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.UpdateTeamPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	team := &organizationv1.Team{}
	if err := s.client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, team); err != nil {
		log.V(1).Info("Failed to get team resource", "error", err)
		return &model.UpdateTeamPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	_, err := s.client.CreateOrUpdate(ctx, team, func() error {
		if input.Email != nil {
			team.Spec.Email = *input.Email
		}
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to update team resource")
		return &model.UpdateTeamPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	log.V(0).Info("Updated team")
	return &model.UpdateTeamPayload{
		Accepted: true,
		Errors:   []model.MutationError{},
	}, nil
}

func (s *teamK8sService) AddTeamMember(ctx context.Context, ref ResourceRef, member model.MemberInput) (*model.AddTeamMemberPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "AddTeamMember", "resourceName", ref.Name, "namespace", ref.Namespace, "memberEmail", member.Email)

	if err := authorizeUpdateTeam(ctx, ref.Group, ref.TeamName); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.AddTeamMemberPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	team := &organizationv1.Team{}
	if err := s.client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, team); err != nil {
		log.V(1).Info("Failed to get team resource", "error", err)
		return &model.AddTeamMemberPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	_, err := s.client.CreateOrUpdate(ctx, team, func() error {
		team.Spec.Members = append(team.Spec.Members, organizationv1.Member{
			Name:  member.Name,
			Email: member.Email,
		})
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to add team member")
		return &model.AddTeamMemberPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	log.V(0).Info("Added team member")
	return &model.AddTeamMemberPayload{
		Errors: []model.MutationError{},
	}, nil
}

func (s *teamK8sService) RemoveTeamMember(ctx context.Context, ref ResourceRef, memberEmail string) (*model.RemoveTeamMemberPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "RemoveTeamMember", "resourceName", ref.Name, "namespace", ref.Namespace, "memberEmail", memberEmail)

	if err := authorizeUpdateTeam(ctx, ref.Group, ref.TeamName); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.RemoveTeamMemberPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	team := &organizationv1.Team{}
	if err := s.client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, team); err != nil {
		log.V(1).Info("Failed to get team resource", "error", err)
		return &model.RemoveTeamMemberPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	_, err := s.client.CreateOrUpdate(ctx, team, func() error {
		filtered := make([]organizationv1.Member, 0, len(team.Spec.Members))
		for _, m := range team.Spec.Members {
			if m.Email != memberEmail {
				filtered = append(filtered, m)
			}
		}
		team.Spec.Members = filtered
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to remove team member")
		return &model.RemoveTeamMemberPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	log.V(0).Info("Removed team member")
	return &model.RemoveTeamMemberPayload{
		Errors: []model.MutationError{},
	}, nil
}

func (s *teamK8sService) RotateTeamToken(ctx context.Context, ref ResourceRef) (*model.RotateTeamTokenPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "RotateTeamToken", "resourceName", ref.Name, "namespace", ref.Namespace)

	if err := authorizeUpdateTeam(ctx, ref.Group, ref.TeamName); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.RotateTeamTokenPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	team := &organizationv1.Team{}
	if err := s.client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, team); err != nil {
		log.V(1).Info("Failed to get team resource", "error", err)
		return &model.RotateTeamTokenPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	_, err := s.client.CreateOrUpdate(ctx, team, func() error {
		team.Spec.Secret = rotateKeyword
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to trigger team token rotation")
		return &model.RotateTeamTokenPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	log.V(0).Info("Triggered team token rotation")
	return &model.RotateTeamTokenPayload{
		Accepted: true,
		Errors:   []model.MutationError{},
	}, nil
}

func toK8sMembers(members []model.MemberInput) []organizationv1.Member {
	result := make([]organizationv1.Member, len(members))
	for i, m := range members {
		result[i] = organizationv1.Member{
			Name:  m.Name,
			Email: m.Email,
		}
	}
	return result
}

// ----- Application -----

type applicationK8sService struct {
	client cc.ScopedClient
}

// NewApplicationK8sService creates an ApplicationService backed by Kubernetes.
func NewApplicationK8sService(c cc.ScopedClient) ApplicationService {
	return &applicationK8sService{client: c}
}

func (s *applicationK8sService) RotateApplicationSecret(ctx context.Context, ref ResourceRef) (*model.RotateApplicationSecretPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "RotateApplicationSecret", "resourceName", ref.Name, "namespace", ref.Namespace)

	if err := authorizeApplicationAction(ctx, ref.TeamName); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.RotateApplicationSecretPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	app := &applicationv1.Application{}
	if err := s.client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, app); err != nil {
		log.V(1).Info("Failed to get application resource", "error", err)
		return &model.RotateApplicationSecretPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	_, err := s.client.CreateOrUpdate(ctx, app, func() error {
		app.Spec.Secret = rotateKeyword
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to trigger application secret rotation")
		return &model.RotateApplicationSecretPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	log.V(0).Info("Triggered application secret rotation")
	return &model.RotateApplicationSecretPayload{
		Accepted: true,
		Errors:   []model.MutationError{},
	}, nil
}

// ----- Approval -----

type approvalK8sService struct {
	client cc.ScopedClient
}

// NewApprovalK8sService creates an ApprovalService backed by Kubernetes.
func NewApprovalK8sService(c cc.ScopedClient) ApprovalService {
	return &approvalK8sService{client: c}
}

// mapActionToApprovalAction maps a GraphQL ApprovalAction to the CRD ApprovalAction.
func mapActionToApprovalAction(action model.ApprovalAction) (approvalv1.ApprovalAction, error) {
	switch strings.ToUpper(string(action)) {
	case "ALLOW":
		return approvalv1.ApprovalActionAllow, nil
	case "DENY":
		return approvalv1.ApprovalActionDeny, nil
	case "SUSPEND":
		return approvalv1.ApprovalActionSuspend, nil
	case "RESUME":
		return approvalv1.ApprovalActionResume, nil
	default:
		return "", fmt.Errorf("unknown approval action: %s", action)
	}
}

// findTargetState looks up the target state for a given action from the available transitions on the CRD.
func findTargetState(transitions approvalv1.AvailableTransitions, action approvalv1.ApprovalAction) (approvalv1.ApprovalState, error) {
	for _, t := range transitions {
		if t.Action == action {
			return t.To, nil
		}
	}
	return "", fmt.Errorf("action %q is not available for the current state", action)
}

// buildDecision creates a CRD Decision from the authenticated viewer context and the resulting state.
func buildDecision(ctx context.Context, input model.DecisionInput, resultingState approvalv1.ApprovalState) approvalv1.Decision {
	now := metav1.Now()
	v := viewer.FromContext(ctx)

	d := approvalv1.Decision{
		Timestamp:      &now,
		ResultingState: resultingState,
	}
	if v != nil {
		d.Name = v.UserName
		d.Email = v.UserEmail
	}
	if input.Comment != nil {
		d.Comment = *input.Comment
	}
	return d
}

func (s *approvalK8sService) DecideApprovalRequest(ctx context.Context, ref ResourceRef, input model.DecisionInput) (*model.DecideApprovalRequestPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "DecideApprovalRequest", "resourceName", ref.Name, "namespace", ref.Namespace, "action", input.Action)

	if err := authorizeApprovalAction(ctx, ref.TeamName); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.DecideApprovalRequestPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	ar := &approvalv1.ApprovalRequest{}
	if err := s.client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, ar); err != nil {
		log.V(1).Info("Failed to get approval request resource", "error", err)
		return &model.DecideApprovalRequestPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	action, err := mapActionToApprovalAction(input.Action)
	if err != nil {
		log.V(1).Info("Invalid approval action", "error", err)
		return &model.DecideApprovalRequestPayload{
			Errors: []model.MutationError{{Code: model.ErrorCodeValidationFailed, Message: err.Error()}},
		}, nil
	}

	targetState, err := findTargetState(ar.Status.AvailableTransitions, action)
	if err != nil {
		log.V(1).Info("Transition not available", "currentState", ar.Spec.State, "requestedAction", action)
		return &model.DecideApprovalRequestPayload{
			Errors: []model.MutationError{{Code: model.ErrorCodePreconditionFailed, Message: err.Error()}},
		}, nil
	}

	decision := buildDecision(ctx, input, targetState)

	_, updateErr := s.client.CreateOrUpdate(ctx, ar, func() error {
		ar.Spec.State = targetState
		ar.Spec.Decisions = append(ar.Spec.Decisions, decision)
		return nil
	})
	if updateErr != nil {
		log.Error(updateErr, "Failed to update approval request", "targetState", targetState)
		return &model.DecideApprovalRequestPayload{
			Errors: []model.MutationError{k8sToMutationError(updateErr)},
		}, nil
	}

	log.V(0).Info("Decided approval request", "targetState", targetState, "decidedByName", decision.Name, "decidedByEmail", decision.Email)
	return &model.DecideApprovalRequestPayload{
		Accepted: true,
		Errors:   []model.MutationError{},
	}, nil
}

func (s *approvalK8sService) DecideApproval(ctx context.Context, ref ResourceRef, input model.DecisionInput) (*model.DecideApprovalPayload, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("operation", "DecideApproval", "resourceName", ref.Name, "namespace", ref.Namespace, "action", input.Action)

	if err := authorizeApprovalAction(ctx, ref.TeamName); err != nil {
		log.V(1).Info("Authorization denied", "reason", err.Error())
		return &model.DecideApprovalPayload{
			Errors: []model.MutationError{forbiddenError(err.Error())},
		}, nil
	}

	approval := &approvalv1.Approval{}
	if err := s.client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, approval); err != nil {
		log.V(1).Info("Failed to get approval resource", "error", err)
		return &model.DecideApprovalPayload{
			Errors: []model.MutationError{k8sToMutationError(err)},
		}, nil
	}

	action, err := mapActionToApprovalAction(input.Action)
	if err != nil {
		log.V(1).Info("Invalid approval action", "error", err)
		return &model.DecideApprovalPayload{
			Errors: []model.MutationError{{Code: model.ErrorCodeValidationFailed, Message: err.Error()}},
		}, nil
	}

	targetState, err := findTargetState(approval.Status.AvailableTransitions, action)
	if err != nil {
		log.V(1).Info("Transition not available", "currentState", approval.Spec.State, "requestedAction", action)
		return &model.DecideApprovalPayload{
			Errors: []model.MutationError{{Code: model.ErrorCodePreconditionFailed, Message: err.Error()}},
		}, nil
	}

	decision := buildDecision(ctx, input, targetState)

	_, updateErr := s.client.CreateOrUpdate(ctx, approval, func() error {
		approval.Spec.State = targetState
		approval.Spec.Decisions = append(approval.Spec.Decisions, decision)
		return nil
	})
	if updateErr != nil {
		log.Error(updateErr, "Failed to update approval", "targetState", targetState)
		return &model.DecideApprovalPayload{
			Errors: []model.MutationError{k8sToMutationError(updateErr)},
		}, nil
	}

	log.V(0).Info("Decided approval", "targetState", targetState, "decidedByName", decision.Name, "decidedByEmail", decision.Email)
	return &model.DecideApprovalPayload{
		Accepted: true,
		Errors:   []model.MutationError{},
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// forbiddenError creates a MutationError with FORBIDDEN code.
func forbiddenError(msg string) model.MutationError {
	return model.MutationError{
		Code:    model.ErrorCodeForbidden,
		Message: msg,
	}
}

// k8sToMutationError converts a Kubernetes API error to a MutationError with
// an appropriate error code based on the error type.
// It unwraps wrapped errors (e.g. from pkg/errors) to find the underlying
// StatusError for proper classification.
func k8sToMutationError(err error) model.MutationError {
	if err == nil {
		return model.MutationError{}
	}

	// Unwrap to find the underlying StatusError, since the scoped client
	// wraps K8s errors with pkg/errors.Wrapf which may not be transparent
	// to apierrors type checks.
	apiErr := unwrapStatusError(err)
	if apiErr == nil {
		return model.MutationError{
			Code:    model.ErrorCodePreconditionFailed,
			Message: "internal error while processing request",
		}
	}

	switch {
	case apierrors.IsNotFound(apiErr):
		return model.MutationError{
			Code:    model.ErrorCodeNotFound,
			Message: "resource not found",
		}
	case apierrors.IsConflict(apiErr):
		return model.MutationError{
			Code:    model.ErrorCodeConflict,
			Message: "resource was modified concurrently, please retry",
		}
	case apierrors.IsForbidden(apiErr):
		return model.MutationError{
			Code:    model.ErrorCodeForbidden,
			Message: "operation forbidden by cluster policy",
		}
	case apierrors.IsInvalid(apiErr):
		return model.MutationError{
			Code:    model.ErrorCodeValidationFailed,
			Message: fmt.Sprintf("resource validation failed: %s", extractCauses(apiErr)),
		}
	case apierrors.IsAlreadyExists(apiErr):
		return model.MutationError{
			Code:    model.ErrorCodeConflict,
			Message: "resource already exists",
		}
	default:
		return model.MutationError{
			Code:    model.ErrorCodePreconditionFailed,
			Message: "internal error while processing request",
		}
	}
}

// unwrapStatusError traverses the error chain to find a *apierrors.StatusError.
// It handles both standard Unwrap() chains and pkg/errors Cause() chains.
func unwrapStatusError(err error) *apierrors.StatusError {
	for e := err; e != nil; {
		var statusErr *apierrors.StatusError
		if ok := errors.As(e, &statusErr); ok {
			return statusErr
		}
		// Support pkg/errors Cause() interface
		type causer interface {
			Cause() error
		}
		if c, ok := e.(causer); ok {
			e = c.Cause()
		} else {
			break
		}
	}
	return nil
}

// extractCauses extracts validation failure causes from a StatusError, providing
// user-friendly messages without exposing internal K8s implementation details.
func extractCauses(statusErr *apierrors.StatusError) string {
	if statusErr == nil {
		return "validation failed"
	}
	if statusErr.ErrStatus.Details == nil || len(statusErr.ErrStatus.Details.Causes) == 0 {
		return "validation failed"
	}

	messages := make([]string, 0, len(statusErr.ErrStatus.Details.Causes))
	for _, cause := range statusErr.ErrStatus.Details.Causes {
		if cause.Field != "" {
			messages = append(messages, fmt.Sprintf("%s: %s", cause.Field, cause.Message))
		} else {
			messages = append(messages, cause.Message)
		}
	}
	return strings.Join(messages, "; ")
}
