// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ types.ObjectStatus = &ObjectStatusResponse{}

type ObjectStatusResponse struct {
	Gone            bool                  `json:"-"`
	OverallStatus   types.OverallStatus   `json:"overallStatus"`
	ProcessingState types.ProcessingState `json:"processingState"`
	Errors          []types.StatusInfo    `json:"errors"`
	Warnings        []types.StatusInfo    `json:"warnings"`
	Info            []types.StatusInfo    `json:"info"`
}

func (o *ObjectStatusResponse) GetOverallStatus() types.OverallStatus {
	return o.OverallStatus
}

func (o *ObjectStatusResponse) GetProcessingState() types.ProcessingState {
	return o.ProcessingState
}

func (o *ObjectStatusResponse) HasErrors() bool {
	return len(o.Errors) > 0
}

func (o *ObjectStatusResponse) HasWarnings() bool {
	return len(o.Warnings) > 0
}

func (o *ObjectStatusResponse) HasInfo() bool {
	return len(o.Info) > 0
}

func (o *ObjectStatusResponse) IsGone() bool {
	return o.Gone
}

func (o *ObjectStatusResponse) GetErrors() []types.StatusInfo {
	return o.Errors
}

func (o *ObjectStatusResponse) GetInfo() []types.StatusInfo {
	return o.Info
}

func (o *ObjectStatusResponse) GetWarnings() []types.StatusInfo {
	return o.Warnings
}
