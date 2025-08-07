// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"io"

	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ types.ObjectStatus = &ObjectStatusResponse{}

type ObjectStatusResponse struct {
}

func (o *ObjectStatusResponse) OverallStatus() string {
	return "unknown"
}

func (o *ObjectStatusResponse) ProcessingState() string {
	return "unknown"
}

func (o *ObjectStatusResponse) HasErrors() bool {
	return false
}

func (o *ObjectStatusResponse) HasWarnings() bool {
	return false
}

func (o *ObjectStatusResponse) HasInfo() bool {
	return false
}

func (o *ObjectStatusResponse) Print(w io.Writer) {
	// Group errors, warnings and infos by resource
	// Print using a structured format and "eye-catchers" ❌ ⚠️ ℹ️
}
