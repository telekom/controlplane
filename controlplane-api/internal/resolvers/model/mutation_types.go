// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// TeamMutationResult is the response type for team mutations.
type TeamMutationResult struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	Namespace    *string `json:"namespace,omitempty"`
	ResourceName *string `json:"resourceName,omitempty"`
}

// CreateTeamInput is the input for creating a new team.
type CreateTeamInput struct {
	Environment string            `json:"environment"`
	Group       string            `json:"group"`
	Name        string            `json:"name"`
	Email       string            `json:"email"`
	Members     []MemberInput     `json:"members"`
	Category    TeamCategoryInput `json:"category"`
}

// UpdateTeamInput is the input for updating an existing team.
type UpdateTeamInput struct {
	Environment string             `json:"environment"`
	Group       string             `json:"group"`
	Name        string             `json:"name"`
	Email       *string            `json:"email,omitempty"`
	Members     []MemberInput      `json:"members,omitempty"`
	Category    *TeamCategoryInput `json:"category,omitempty"`
}

// MemberInput represents a team member in mutation inputs.
type MemberInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// RotateApplicationSecretInput is the input for rotating an application's client secret.
type RotateApplicationSecretInput struct {
	Environment string `json:"environment"`
	Team        string `json:"team"`
	Name        string `json:"name"`
}

// ApplicationMutationResult is the response type for application mutations.
type ApplicationMutationResult struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	Namespace    *string `json:"namespace,omitempty"`
	ResourceName *string `json:"resourceName,omitempty"`
}

// RotateTeamTokenInput is the input for rotating a team's token.
type RotateTeamTokenInput struct {
	Environment string `json:"environment"`
	Group       string `json:"group"`
	Name        string `json:"name"`
}

// TeamCategoryInput represents the team category enum for mutations.
type TeamCategoryInput string

const (
	TeamCategoryInputCustomer       TeamCategoryInput = "CUSTOMER"
	TeamCategoryInputInfrastructure TeamCategoryInput = "INFRASTRUCTURE"
)
