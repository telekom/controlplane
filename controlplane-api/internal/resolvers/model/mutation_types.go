// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
)

// ──────────────────────────────────────────────────────────────────────────────
// Shared types
// ──────────────────────────────────────────────────────────────────────────────

// MutationError represents a domain error returned in mutation payloads.
type MutationError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Field   *string   `json:"field,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Input types
// ──────────────────────────────────────────────────────────────────────────────

// CreateTeamInput is the input for creating a new team.
type CreateTeamInput struct {
	Environment string        `json:"environment"`
	Group       string        `json:"group"`
	Name        string        `json:"name"`
	Email       string        `json:"email"`
	Members     []MemberInput `json:"members"`
	DisplayName *string       `json:"displayName,omitempty"`
	Description *string       `json:"description,omitempty"`
}

// UpdateTeamInput is the input for updating team metadata.
type UpdateTeamInput struct {
	TeamID      int     `json:"teamId"`
	Email       *string `json:"email,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	Description *string `json:"description,omitempty"`
}

// DeleteTeamInput is the input for deleting a team.
type DeleteTeamInput struct {
	TeamID int `json:"teamId"`
}

// CreateGroupInput is the input for creating a new group.
type CreateGroupInput struct {
	Environment string  `json:"environment"`
	Name        string  `json:"name"`
	DisplayName string  `json:"displayName"`
	Description *string `json:"description,omitempty"`
}

// UpdateGroupInput is the input for updating group metadata.
type UpdateGroupInput struct {
	GroupID     int     `json:"groupId"`
	DisplayName *string `json:"displayName,omitempty"`
	Description *string `json:"description,omitempty"`
}

// DeleteGroupInput is the input for deleting a group.
type DeleteGroupInput struct {
	GroupID int `json:"groupId"`
}

// MemberInput represents a team member in mutation inputs.
type MemberInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// DecisionInput represents the decision for approval mutations.
type DecisionInput struct {
	Action  ApprovalAction `json:"action"`
	Comment *string        `json:"comment,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Payload types — Team
// ──────────────────────────────────────────────────────────────────────────────

// CreateTeamPayload is the response for createTeam.
type CreateTeamPayload struct {
	Team     *ent.Team       `json:"team,omitempty"`
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}

// UpdateTeamPayload is the response for updateTeam.
type UpdateTeamPayload struct {
	Team     *ent.Team       `json:"team,omitempty"`
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}

// DeleteTeamPayload is the response for deleteTeam.
type DeleteTeamPayload struct {
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}

// AddTeamMemberPayload is the response for addTeamMember.
type AddTeamMemberPayload struct {
	Team   *ent.Team       `json:"team,omitempty"`
	Errors []MutationError `json:"errors"`
}

// RemoveTeamMemberPayload is the response for removeTeamMember.
type RemoveTeamMemberPayload struct {
	Team   *ent.Team       `json:"team,omitempty"`
	Errors []MutationError `json:"errors"`
}

// RotateTeamTokenPayload is the response for rotateTeamToken.
type RotateTeamTokenPayload struct {
	Team     *ent.Team       `json:"team,omitempty"`
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Payload types — Group
// ──────────────────────────────────────────────────────────────────────────────

// CreateGroupPayload is the response for createGroup.
type CreateGroupPayload struct {
	Group    *ent.Group      `json:"group,omitempty"`
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}

// UpdateGroupPayload is the response for updateGroup.
type UpdateGroupPayload struct {
	Group    *ent.Group      `json:"group,omitempty"`
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}

// DeleteGroupPayload is the response for deleteGroup.
type DeleteGroupPayload struct {
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Payload types — Application
// ──────────────────────────────────────────────────────────────────────────────

// RotateApplicationSecretPayload is the response for rotateApplicationSecret.
type RotateApplicationSecretPayload struct {
	Application *ent.Application `json:"application,omitempty"`
	Accepted    bool             `json:"accepted"`
	Errors      []MutationError  `json:"errors"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Payload types — Approval
// ──────────────────────────────────────────────────────────────────────────────

// DecideApprovalRequestPayload is the response for decideApprovalRequest.
type DecideApprovalRequestPayload struct {
	ApprovalRequest *ent.ApprovalRequest `json:"approvalRequest,omitempty"`
	Accepted        bool                 `json:"accepted"`
	Errors          []MutationError      `json:"errors"`
}

// DecideApprovalPayload is the response for decideApproval.
type DecideApprovalPayload struct {
	Approval *ent.Approval   `json:"approval,omitempty"`
	Accepted bool            `json:"accepted"`
	Errors   []MutationError `json:"errors"`
}
