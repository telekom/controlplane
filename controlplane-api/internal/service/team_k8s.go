// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

