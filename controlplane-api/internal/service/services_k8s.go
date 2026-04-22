// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"strings"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// teamScopedNamespace returns the K8s namespace for resources scoped to a team: <environment>--<team>.
func teamScopedNamespace(environment, team string) string {
	return environment + "--" + team
}

// ----- Team -----

type teamK8sService struct {
	client client.Client
}

// NewTeamK8sService creates a TeamService backed by Kubernetes.
func NewTeamK8sService(c client.Client) TeamService {
	return &teamK8sService{client: c}
}

func (s *teamK8sService) CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.TeamMutationResult, error) {
	if err := authorizeCreateTeam(ctx, input.Group); err != nil {
		return nil, err
	}

	resourceName := teamResourceName(input.Group, input.Name)
	namespace := input.Environment

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	team := &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
	}

	_, err := scopedClient.CreateOrUpdate(ctx, team, func() error {
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
		return nil, mapK8sError(err)
	}

	return &model.TeamMutationResult{
		Success:      true,
		Message:      "team created successfully",
		Namespace:    &namespace,
		ResourceName: &resourceName,
	}, nil
}

func (s *teamK8sService) UpdateTeam(ctx context.Context, input model.UpdateTeamInput) (*model.TeamMutationResult, error) {
	if err := authorizeUpdateTeam(ctx, input.Group, input.Name); err != nil {
		return nil, err
	}

	resourceName := teamResourceName(input.Group, input.Name)
	namespace := input.Environment

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	team := &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
	}

	_, err := scopedClient.CreateOrUpdate(ctx, team, func() error {
		if input.Email != nil {
			team.Spec.Email = *input.Email
		}
		if input.Members != nil {
			team.Spec.Members = toK8sMembers(input.Members)
		}
		return nil
	})
	if err != nil {
		return nil, mapK8sError(err)
	}

	return &model.TeamMutationResult{
		Success:      true,
		Message:      "team updated successfully",
		Namespace:    &namespace,
		ResourceName: &resourceName,
	}, nil
}

func (s *teamK8sService) RotateTeamToken(ctx context.Context, input model.RotateTeamTokenInput) (*model.TeamMutationResult, error) {
	if err := authorizeUpdateTeam(ctx, input.Group, input.Name); err != nil {
		return nil, err
	}

	resourceName := teamResourceName(input.Group, input.Name)
	namespace := input.Environment

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	team := &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
	}

	_, err := scopedClient.CreateOrUpdate(ctx, team, func() error {
		team.Spec.Secret = "rotate"
		return nil
	})
	if err != nil {
		return nil, mapK8sError(err)
	}

	return &model.TeamMutationResult{
		Success:      true,
		Message:      "team token rotation initiated",
		Namespace:    &namespace,
		ResourceName: &resourceName,
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
	client client.Client
}

// NewApplicationK8sService creates an ApplicationService backed by Kubernetes.
func NewApplicationK8sService(c client.Client) ApplicationService {
	return &applicationK8sService{client: c}
}

func (s *applicationK8sService) RotateApplicationSecret(ctx context.Context, input model.RotateApplicationSecretInput) (*model.RotateApplicationSecretResult, error) {
	if err := authorizeApplicationAction(ctx, input.Team); err != nil {
		return nil, err
	}

	namespace := teamScopedNamespace(input.Environment, input.Team)
	resourceName := input.Name

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	app := &applicationv1.Application{}
	err := scopedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: resourceName}, app)
	if err != nil {
		return nil, mapK8sError(err)
	}

	_, err = scopedClient.CreateOrUpdate(ctx, app, func() error {
		app.Spec.Secret = "rotate"
		return nil
	})
	if err != nil {
		return nil, mapK8sError(err)
	}

	return &model.RotateApplicationSecretResult{
		Success:      true,
		Message:      "application secret rotation initiated",
		Namespace:    &namespace,
		ResourceName: &resourceName,
	}, nil
}

// ----- Approval -----

type approvalK8sService struct {
	client client.Client
}

// NewApprovalK8sService creates an ApprovalService backed by Kubernetes.
func NewApprovalK8sService(c client.Client) ApprovalService {
	return &approvalK8sService{client: c}
}

// mapActionToApprovalAction maps a GraphQL action string (uppercase) to the CRD ApprovalAction (title-case).
func mapActionToApprovalAction(action string) (approvalv1.ApprovalAction, error) {
	switch strings.ToUpper(action) {
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

// buildDecision creates a CRD Decision from the GraphQL input and the resulting state.
func buildDecision(input model.DecisionInput, resultingState approvalv1.ApprovalState) approvalv1.Decision {
	now := metav1.Now()
	d := approvalv1.Decision{
		Name:           input.Name,
		Timestamp:      &now,
		ResultingState: resultingState,
	}
	d.Email = input.Email
	if input.Comment != nil {
		d.Comment = *input.Comment
	}
	return d
}

func (s *approvalK8sService) DecideApprovalRequest(ctx context.Context, input model.DecideApprovalRequestInput) (*model.ApprovalMutationResult, error) {
	namespace := teamScopedNamespace(input.Environment, input.Team)
	resourceName := input.Name

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	ar := &approvalv1.ApprovalRequest{}
	err := scopedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: resourceName}, ar)
	if err != nil {
		return nil, mapK8sError(err)
	}

	if err := authorizeApprovalAction(ctx, ar.Spec.Decider.TeamName); err != nil {
		return nil, err
	}

	crdAction, err := mapActionToApprovalAction(input.Action)
	if err != nil {
		return nil, err
	}

	targetState, err := findTargetState(ar.Status.AvailableTransitions, crdAction)
	if err != nil {
		return nil, err
	}

	_, err = scopedClient.CreateOrUpdate(ctx, ar, func() error {
		ar.Spec.Decisions = append(ar.Spec.Decisions, buildDecision(input.Decision, targetState))
		ar.Spec.State = targetState
		return nil
	})
	if err != nil {
		return nil, mapK8sError(err)
	}

	newState := string(targetState)
	return &model.ApprovalMutationResult{
		Success:      true,
		Message:      "approval request decision applied",
		NewState:     &newState,
		Namespace:    &namespace,
		ResourceName: &resourceName,
	}, nil
}

func (s *approvalK8sService) DecideApproval(ctx context.Context, input model.DecideApprovalInput) (*model.ApprovalMutationResult, error) {
	namespace := teamScopedNamespace(input.Environment, input.Team)
	resourceName := input.Name

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	approval := &approvalv1.Approval{}
	err := scopedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: resourceName}, approval)
	if err != nil {
		return nil, mapK8sError(err)
	}

	if err := authorizeApprovalAction(ctx, approval.Spec.Decider.TeamName); err != nil {
		return nil, err
	}

	crdAction, err := mapActionToApprovalAction(input.Action)
	if err != nil {
		return nil, err
	}

	targetState, err := findTargetState(approval.Status.AvailableTransitions, crdAction)
	if err != nil {
		return nil, err
	}

	_, err = scopedClient.CreateOrUpdate(ctx, approval, func() error {
		approval.Spec.Decisions = append(approval.Spec.Decisions, buildDecision(input.Decision, targetState))
		approval.Spec.State = targetState
		return nil
	})
	if err != nil {
		return nil, mapK8sError(err)
	}

	newState := string(targetState)
	return &model.ApprovalMutationResult{
		Success:      true,
		Message:      "approval decision applied",
		NewState:     &newState,
		Namespace:    &namespace,
		ResourceName: &resourceName,
	}, nil
}
