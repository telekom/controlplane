// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import "io"

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

type ObjectStatus interface {
	OverallStatus() string
	ProcessingState() string
	HasErrors() bool
	HasWarnings() bool
	HasInfo() bool
	Print(io.Writer)
}
