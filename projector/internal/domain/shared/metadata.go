// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package shared provides cross-module helpers for the projector
// domain layer, including metadata construction, status extraction, and
// namespace parsing.
package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// EnvironmentLabelKey is the label key that carries the environment name on
// every CR. Set by the ScopedClient at creation time.
const EnvironmentLabelKey = "cp.ei.telekom.de/environment"

// Metadata holds the common identity and context fields extracted from every
// Kubernetes CR. These fields are stored alongside entity-specific data.
type Metadata struct {
	Namespace   string
	Name        string
	NodeHash    string
	Environment string
}

// NewMetadata constructs a Metadata from the CR's namespace, name, and labels.
// It computes the NodeHash and extracts the Environment label.
func NewMetadata(namespace, name string, labels map[string]string) Metadata {
	return Metadata{
		Namespace:   namespace,
		Name:        name,
		NodeHash:    NodeHash(namespace, name),
		Environment: EnvironmentFromLabels(labels),
	}
}

// NodeHash computes a truncated SHA-256 hash of namespace/name for use as
// an opaque, fixed-length GraphQL node ID. Returns 16 hex chars (64 bits).
func NodeHash(namespace, name string) string {
	input := fmt.Sprintf("%s/%s", namespace, name)
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:8]) // first 8 bytes = 16 hex chars
}

// EnvironmentFromLabels extracts the environment name from the CR label map.
// Returns an empty string if the label is not present or labels is nil.
func EnvironmentFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	return labels[EnvironmentLabelKey]
}
