// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphqlinputs

type GroupWhereInput struct {
	Name *string `json:"name,omitempty"`
}

type TeamWhereInput struct {
	Name         *string           `json:"name,omitempty"`
	HasGroupWith []GroupWhereInput `json:"hasGroupWith,omitempty"`
}
