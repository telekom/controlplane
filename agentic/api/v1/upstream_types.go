// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

type Upstream struct {
	Url    string `json:"url"`
	Weight int    `json:"weight,omitempty"`
}
