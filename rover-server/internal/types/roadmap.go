// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// RoadmapItem represents a single timeline entry in the roadmap
// This matches the RoadmapItem struct in rover/api/v1/roadmap_types.go
type RoadmapItem struct {
	Date        string `json:"date"`
	Title       string `json:"title"`
	Description string `json:"description"`
	TitleUrl    string `json:"titleUrl,omitempty"`
}

// RoadmapRequest represents the request to create/update a roadmap
type RoadmapRequest struct {
	ResourceName string               `json:"resourceName"`
	ResourceType roverv1.ResourceType `json:"resourceType"`
	Items        []RoadmapItem        `json:"items"`
}

// RoadmapResponse represents the response when fetching a roadmap
type RoadmapResponse struct {
	Id           string             `json:"id"`
	Name         string             `json:"name"`
	ResourceName string             `json:"resourceName"`
	ResourceType string             `json:"resourceType"`
	Items        []RoadmapItem      `json:"items"`
	Status       *RoadmapStatusInfo `json:"status,omitempty"`
}

// RoadmapListResponse represents the response when listing roadmaps
type RoadmapListResponse struct {
	Items []RoadmapResponse `json:"items"`
	Links *ResponseLinks    `json:"_links,omitempty"`
}

// RoadmapStatusInfo represents the status information of a roadmap
type RoadmapStatusInfo struct {
	Ready      bool   `json:"ready"`
	Processing bool   `json:"processing"`
	Message    string `json:"message,omitempty"`
}

// ResponseLinks represents pagination links
type ResponseLinks struct {
	Self string `json:"self,omitempty"`
	Next string `json:"next,omitempty"`
}

// GetAllRoadmapsParams represents query parameters for listing roadmaps
type GetAllRoadmapsParams struct {
	Cursor       *string               `query:"cursor"`
	ResourceType *roverv1.ResourceType `query:"resourceType"`
}
