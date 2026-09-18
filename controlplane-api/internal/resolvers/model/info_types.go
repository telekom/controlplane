// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	pkgmodel "github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// ApplicationInfo provides a reduced cross-tenant safe view of an application.
// No navigable edges — traversal terminates here.
type ApplicationInfo struct {
	ID        int                `json:"id"`
	Name      string             `json:"name"`
	Zone      *ent.Zone          `json:"zone"`
	OwnerTeam *pkgmodel.TeamInfo `json:"ownerTeam"`
}

// ApiExposureInfo provides a reduced cross-tenant safe view of an API exposure.
// No navigable edges — traversal terminates here.
type ApiExposureInfo struct {
	ID                   int                     `json:"id"`
	BasePath             string                  `json:"basePath"`
	Visibility           string                  `json:"visibility"`
	Active               *bool                   `json:"active,omitempty"`
	ApiVersion           *string                 `json:"apiVersion,omitempty"`
	Features             []string                `json:"features"`
	Traffic              *pkgmodel.Traffic       `json:"traffic,omitempty"`
	ApprovalConfig       pkgmodel.ApprovalConfig `json:"approvalConfig"`
	OwnerApplicationName string                  `json:"ownerApplicationName"`
	OwnerTeam            *pkgmodel.TeamInfo      `json:"ownerTeam"`
	OwnerApplication     *ApplicationInfo        `json:"ownerApplication"`
}

// ApiSubscriptionInfo provides a reduced cross-tenant safe view of an API subscription.
// No navigable edges — traversal terminates here.
type ApiSubscriptionInfo struct {
	ID                   int                `json:"id"`
	BasePath             string             `json:"basePath"`
	StatusPhase          *string            `json:"statusPhase,omitempty"`
	StatusMessage        *string            `json:"statusMessage,omitempty"`
	OwnerApplicationName string             `json:"ownerApplicationName"`
	OwnerTeam            *pkgmodel.TeamInfo `json:"ownerTeam"`
	OwnerApplication     *ApplicationInfo   `json:"ownerApplication"`
}

func (ApiSubscriptionInfo) IsSubscriptionInfo() {}

func (s *ApiSubscriptionInfo) GetID() int { return s.ID }

func (s *ApiSubscriptionInfo) GetOwnerApplication() *ApplicationInfo { return s.OwnerApplication }

// EventSubscriptionInfo provides a reduced cross-tenant safe view of an event subscription.
// No navigable edges — traversal terminates here.
type EventSubscriptionInfo struct {
	ID                   int                `json:"id"`
	EventType            string             `json:"eventType"`
	DeliveryType         string             `json:"deliveryType"`
	StatusPhase          *string            `json:"statusPhase,omitempty"`
	StatusMessage        *string            `json:"statusMessage,omitempty"`
	OwnerApplicationName string             `json:"ownerApplicationName"`
	OwnerTeam            *pkgmodel.TeamInfo `json:"ownerTeam"`
	OwnerApplication     *ApplicationInfo   `json:"ownerApplication"`
}

func (EventSubscriptionInfo) IsSubscriptionInfo() {}

func (s *EventSubscriptionInfo) GetID() int { return s.ID }

func (s *EventSubscriptionInfo) GetOwnerApplication() *ApplicationInfo { return s.OwnerApplication }

// EventExposureInfo provides a reduced cross-tenant safe view of an event exposure.
// No navigable edges — traversal terminates here.
type EventExposureInfo struct {
	ID                   int                     `json:"id"`
	EventType            string                  `json:"eventType"`
	Visibility           string                  `json:"visibility"`
	Active               *bool                   `json:"active,omitempty"`
	ApprovalConfig       pkgmodel.ApprovalConfig `json:"approvalConfig"`
	OwnerApplicationName string                  `json:"ownerApplicationName"`
	OwnerTeam            *pkgmodel.TeamInfo      `json:"ownerTeam"`
	OwnerApplication     *ApplicationInfo        `json:"ownerApplication"`
}

// AgenticExposureInfo provides a reduced cross-tenant safe view of an agentic exposure.
// No navigable edges — traversal terminates here.
type AgenticExposureInfo struct {
	ID                   int                     `json:"id"`
	BasePath             string                  `json:"basePath"`
	Visibility           string                  `json:"visibility"`
	Variant              string                  `json:"variant"`
	Active               *bool                   `json:"active,omitempty"`
	ApprovalConfig       pkgmodel.ApprovalConfig `json:"approvalConfig"`
	Traffic              *pkgmodel.Traffic       `json:"traffic,omitempty"`
	OwnerApplicationName string                  `json:"ownerApplicationName"`
	OwnerTeam            *pkgmodel.TeamInfo      `json:"ownerTeam"`
	OwnerApplication     *ApplicationInfo        `json:"ownerApplication"`
}

// AgenticSubscriptionInfo provides a reduced cross-tenant safe view of an agentic subscription.
// No navigable edges — traversal terminates here.
type AgenticSubscriptionInfo struct {
	ID                   int                `json:"id"`
	BasePath             string             `json:"basePath"`
	StatusPhase          *string            `json:"statusPhase,omitempty"`
	StatusMessage        *string            `json:"statusMessage,omitempty"`
	OwnerApplicationName string             `json:"ownerApplicationName"`
	OwnerTeam            *pkgmodel.TeamInfo `json:"ownerTeam"`
	OwnerApplication     *ApplicationInfo   `json:"ownerApplication"`
}

func (AgenticSubscriptionInfo) IsSubscriptionInfo() {}

func (s *AgenticSubscriptionInfo) GetID() int { return s.ID }

func (s *AgenticSubscriptionInfo) GetOwnerApplication() *ApplicationInfo { return s.OwnerApplication }
