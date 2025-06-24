// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

type Security struct {
	Authentication *Authentication `json:"authentication,omitempty"`
}

type Authentication struct {
	OAuth2    *OAuth2    `json:"oauth2,omitempty"`
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
}

type OAuth2 struct {
	Scopes []string `json:"oauth2Scopes,omitempty"`
}

type BasicAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}
