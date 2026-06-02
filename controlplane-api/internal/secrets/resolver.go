// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

const obfuscatedValue = "**********"

// Resolver resolves secret references ($<ref>) to their plaintext values
// just-in-time. It checks the caller's access level and returns obfuscated
// values for callers without read-write access.
type Resolver struct {
	api secretsapi.SecretsApi
}

// NewResolver creates a Resolver backed by the given SecretsApi.
func NewResolver(api secretsapi.SecretsApi) *Resolver {
	return &Resolver{api: api}
}

// Resolve checks whether value is a secret-manager reference ($<...>) and
// resolves it. If the caller's BusinessContext has AccessType "obfuscated",
// the value is masked instead. Non-reference values are returned unchanged.
//
// Security invariants:
//   - Obfuscated callers never see plaintext secrets.
//   - Resolved secret values are never logged.
//   - Access to secret fields is audit-logged at V(0).
func (r *Resolver) Resolve(ctx context.Context, value *string, fieldName string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	ref, isRef := secretsapi.FromRef(*value)
	if !isRef {
		// Not a secret reference — return as-is (backwards compatibility
		// for values that haven't been migrated to secret-manager yet).
		return value, nil
	}

	log := logr.FromContextOrDiscard(ctx)

	// Obfuscated callers must never receive plaintext secrets.
	if security.IsObfuscated(ctx) {
		log.V(1).Info("Secret field requested by obfuscated caller, masking", "field", fieldName)
		masked := obfuscatedValue
		return &masked, nil
	}

	log.Info("Resolving secret field", "field", fieldName)

	resolved, err := r.api.Get(ctx, ref)
	if err != nil {
		log.Error(err, "Failed to resolve secret from secret-manager", "field", fieldName)
		return nil, fmt.Errorf("failed to resolve secret %s", fieldName)
	}

	return &resolved, nil
}
