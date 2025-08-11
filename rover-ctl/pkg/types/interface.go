// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

type ObjectMetadata struct {
	Name string `yaml:"name" json:"name" validate:"required"`
}

type Object interface {
	GetApiVersion() string
	GetKind() string
	SetApiVersion(version string)
	SetKind(kind string)
	GetName() string
	SetName(name string)

	GetContent() map[string]any
	SetContent(map[string]any)

	GetProperty(name string) any
	SetProperty(name string, value any)
}

type ObjectRef struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

type StatusInfo struct {
	Cause    string    `json:"cause"`
	Message  string    `json:"message"`
	Details  string    `json:"details,omitempty"`
	Resource ObjectRef `json:"resource"`
}

type StateInfoContainer interface {
	GetErrors() []StatusInfo
	GetInfo() []StatusInfo
	GetWarnings() []StatusInfo
}

type ObjectStatus interface {
	StateInfoContainer
	GetOverallStatus() string
	GetProcessingState() string
	HasErrors() bool
	HasWarnings() bool
	HasInfo() bool
	IsGone() bool
}
