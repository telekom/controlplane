// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// Buffering configures Kong request/response body buffering for a route.
// By default, Kong buffers both request and response bodies before proxying.
type Buffering struct {
	// DisableRequestBuffering disables Kong request body buffering.
	// When true, the request body is streamed directly to the upstream
	// without being buffered first. Useful for large uploads or chunked transfers.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	DisableRequestBuffering bool `json:"disableRequestBuffering,omitempty"`

	// DisableResponseBuffering disables Kong response body buffering.
	// When true, the response body is streamed directly to the client
	// without being buffered first. Useful for SSE or large downloads.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	DisableResponseBuffering bool `json:"disableResponseBuffering,omitempty"`
}
