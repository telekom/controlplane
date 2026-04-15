// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"strings"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type approvalK8sService struct {
	client client.Client
}

// NewApprovalK8sService creates an ApprovalService backed by Kubernetes.
func NewApprovalK8sService(c client.Client) ApprovalService {
	return &approvalK8sService{client: c}
}

// approvalNamespace returns the K8s namespace for approval resources: <environment>--<team>.
func approvalNamespace(environment, team string) string {
	return environment + "--" + team
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
	namespace := approvalNamespace(input.Environment, input.Team)
	resourceName := input.Name

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	// Fetch existing ApprovalRequest
	ar := &approvalv1.ApprovalRequest{}
	err := scopedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: resourceName}, ar)
	if err != nil {
		return nil, mapK8sError(err)
	}

	// Authorize: admin or viewer's team matches the decider team
	if err := authorizeApprovalAction(ctx, ar.Spec.Decider.TeamName); err != nil {
		return nil, err
	}

	// Map action and find target state from available transitions
	crdAction, err := mapActionToApprovalAction(input.Action)
	if err != nil {
		return nil, err
	}

	targetState, err := findTargetState(ar.Status.AvailableTransitions, crdAction)
	if err != nil {
		return nil, err
	}

	// Update the CRD
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
	namespace := approvalNamespace(input.Environment, input.Team)
	resourceName := input.Name

	scopedClient := cc.NewScopedClient(s.client, input.Environment)

	// Fetch existing Approval
	approval := &approvalv1.Approval{}
	err := scopedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: resourceName}, approval)
	if err != nil {
		return nil, mapK8sError(err)
	}

	// Authorize: admin or viewer's team matches the decider team
	if err := authorizeApprovalAction(ctx, approval.Spec.Decider.TeamName); err != nil {
		return nil, err
	}

	// Map action and find target state from available transitions
	crdAction, err := mapActionToApprovalAction(input.Action)
	if err != nil {
		return nil, err
	}

	targetState, err := findTargetState(approval.Status.AvailableTransitions, crdAction)
	if err != nil {
		return nil, err
	}

	// Update the CRD
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
