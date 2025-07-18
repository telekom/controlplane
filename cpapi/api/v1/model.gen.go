// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package v1 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.4.1 DO NOT EDIT.
package v1

import (
	"time"
)

// Defines values for ApprovalInfoState.
const (
	Granted   ApprovalInfoState = "Granted"
	Pending   ApprovalInfoState = "Pending"
	Rejected  ApprovalInfoState = "Rejected"
	Suspended ApprovalInfoState = "Suspended"
)

// Defines values for StatusConditionStatus.
const (
	False   StatusConditionStatus = "False"
	True    StatusConditionStatus = "True"
	Unknown StatusConditionStatus = "Unknown"
)

// ApprovalInfo defines model for ApprovalInfo.
type ApprovalInfo struct {
	// Message An optional message from the Decider to the Requester.
	Message string `json:"message"`

	// State The current state of the approval
	State ApprovalInfoState `json:"state"`
}

// ApprovalInfoState The current state of the approval
type ApprovalInfoState string

// Error RFC-7807 conform object sent on any error
type Error struct {
	Detail    *string         `json:"detail,omitempty"`
	ErrorCode *string         `json:"errorCode,omitempty"`
	Fields    *[]FieldProblem `json:"fields,omitempty"`
	Instance  *string         `json:"instance,omitempty"`
	Status    *float32        `json:"status,omitempty"`
	Title     string          `json:"title"`
	Type      string          `json:"type"`
}

// FieldProblem defines model for FieldProblem.
type FieldProblem struct {
	Detail *string `json:"detail,omitempty"`
	Path   *string `json:"path,omitempty"`
	Title  string  `json:"title"`
}

// RemoteSubscriptionResponse defines model for RemoteSubscriptionResponse.
type RemoteSubscriptionResponse = Response

// RemoteSubscriptionSpec defines model for RemoteSubscriptionSpec.
type RemoteSubscriptionSpec struct {
	// ApiBasePath The basePath of the API that you want to subscribe to
	ApiBasePath string    `json:"apiBasePath"`
	Requester   Requester `json:"requester"`
	Security    *Security `json:"security,omitempty"`
}

// RemoteSubscriptionStatus RemoteSubscriptionStatus defines the state of RemoteSubscription
type RemoteSubscriptionStatus struct {
	Approval        *ApprovalInfo     `json:"approval,omitempty"`
	ApprovalRequest *ApprovalInfo     `json:"approvalRequest,omitempty"`
	Conditions      []StatusCondition `json:"conditions"`

	// GatewayUrl The URL for the subscribed API.
	GatewayUrl string `json:"gatewayUrl"`
}

// Requester defines model for Requester.
type Requester struct {
	// Application The name of the Application that is used to subscribe. **Note:** This needs to exactly match the application used on the source CP.
	Application string `json:"application"`
	Team        Team   `json:"team"`
}

// Response defines model for Response.
type Response struct {
	Id      string `json:"id"`
	Updated bool   `json:"updated"`
}

// Security defines model for Security.
type Security struct {
	Oauth2 *SecurityOauth2 `json:"oauth2,omitempty"`
}

// SecurityOauth2 defines model for SecurityOauth2.
type SecurityOauth2 struct {
	Scopes *[]string `json:"scopes,omitempty"`
}

// StatusCondition **Note:** This is a 1:1 copy of the StatusCondition from the Kubernetes API. Condition contains details for one aspect of the current state of this API Resource
type StatusCondition struct {
	// LastTransitionTime lastTransitionTime is the last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
	LastTransitionTime time.Time `json:"lastTransitionTime"`

	// Message message is a human readable message indicating details about the transition.
	// This may be an empty string.
	Message string `json:"message"`

	// ObservedGeneration observedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
	// with respect to the current state of the instance.
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`

	// Reason reason contains a programmatic identifier indicating the reason for the condition's last transition.
	// Producers of specific condition types may define expected values and meanings for this field,
	// and whether the values are considered a guaranteed API.
	// The value should be a CamelCase string.
	// This field may not be empty.
	Reason string `json:"reason"`

	// Status status of the condition, one of True, False, Unknown.
	Status StatusConditionStatus `json:"status"`

	// Type type of condition in CamelCase or in foo.example.com/CamelCase.
	// ---
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)
	Type string `json:"type"`
}

// StatusConditionStatus status of the condition, one of True, False, Unknown.
type StatusConditionStatus string

// Team defines model for Team.
type Team struct {
	// Email The email address that can be used in case the provider wants to contact the subscribing team.
	Email string `json:"email"`

	// Name The name of the Team that is used to subscribe. **Note:** This needs to exactly match the team used on the source CP.
	Name string `json:"name"`
}

// RemoteSubscriptionId defines model for RemoteSubscriptionId.
type RemoteSubscriptionId = string

// BadRequest RFC-7807 conform object sent on any error
type BadRequest = Error

// Forbidden RFC-7807 conform object sent on any error
type Forbidden = Error

// NotFound RFC-7807 conform object sent on any error
type NotFound = Error

// ServerError RFC-7807 conform object sent on any error
type ServerError = Error

// Unauthorized RFC-7807 conform object sent on any error
type Unauthorized = Error

// UnsupportedMediaType RFC-7807 conform object sent on any error
type UnsupportedMediaType = Error

// CreateOrUpdateRemoteSubscriptionJSONRequestBody defines body for CreateOrUpdateRemoteSubscription for application/json ContentType.
type CreateOrUpdateRemoteSubscriptionJSONRequestBody = RemoteSubscriptionSpec

// UpdateRemoteSubscriptionStatusJSONRequestBody defines body for UpdateRemoteSubscriptionStatus for application/json ContentType.
type UpdateRemoteSubscriptionStatusJSONRequestBody = RemoteSubscriptionStatus
