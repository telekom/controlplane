// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

type Links struct {
	Next string `json:"next"`
	Self string `json:"self"`
}

type ListResponse struct {
	Links Links `json:"_links"`
	Items []any `json:"items"`
}
