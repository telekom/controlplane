// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package log provides helpers for producing log-safe representations of
// Kubernetes objects and for configuring the operator's log verbosity.
package log

import (
	"os"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DisableSanitizationEnvVar is the environment variable operators can set to
// "true" (parsed via strconv.ParseBool) to disable managedFields
// sanitization in SanitizeForLog. It is enabled by default (i.e. unset,
// empty, or unparsable values keep sanitization on).
const DisableSanitizationEnvVar = "DISABLE_MANAGED_FIELDS_SANITIZATION"

// SanitizeForLog returns a deep copy of obj with metadata.managedFields cleared.
//
// managedFields is Kubernetes bookkeeping metadata (tracking which field
// manager last touched each field) that is primarily useful to the API
// server, not to operators reading logs. It can dominate the size of a
// logged object, making log output noisy and harder to read.
//
// The returned object is a copy: the original obj is never mutated, so it
// remains safe to use for reconciliation and persistence (e.g. subsequent
// Update/Status().Update() calls retain their original managedFields).
//
// Sanitization can be disabled by setting DisableSanitizationEnvVar to
// "true", in which case obj is returned unmodified (no copy is made).
func SanitizeForLog(obj client.Object) client.Object {
	if obj == nil {
		return nil
	}

	if sanitizationDisabled() {
		return obj
	}

	sanitized, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		// Should not happen for well-formed client.Object implementations,
		// since DeepCopyObject() is expected to preserve the concrete type.
		return obj
	}

	sanitized.SetManagedFields(nil)
	return sanitized
}

// sanitizationDisabled reports whether DisableSanitizationEnvVar is set to a
// truthy value. Unset, empty, or unparsable values are treated as false
// (sanitization stays enabled).
func sanitizationDisabled() bool {
	disabled, err := strconv.ParseBool(os.Getenv(DisableSanitizationEnvVar))
	if err != nil {
		return false
	}
	return disabled
}
