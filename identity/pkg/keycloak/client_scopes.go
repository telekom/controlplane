// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak/protocolmappers"
)

// ManagedClientScopeName is the name of the Keycloak client scope managed by
// the controller for custom claim injection (HardcodedClaim and SessionNote).
const ManagedClientScopeName = "controlplane-claims"

// ConfigureClientScopes ensures that a Keycloak client scope with protocol
// mappers for the configured claims exists and is assigned as a realm default
// scope. When claims is empty any previously managed scope is removed.
func (k *keycloakService) ConfigureClientScopes(ctx context.Context, realmName string, claims []identityv1.ClaimConfig) error {
	logger := logr.FromContextOrDiscard(ctx).WithValues("realm", realmName, "scope", ManagedClientScopeName)

	existing, existingID, err := k.findManagedClientScope(ctx, realmName)
	if err != nil {
		return err
	}

	// No claims desired — clean up if the managed scope exists.
	if len(claims) == 0 {
		logger.Info("cleaning up client-scope")
		if existing != nil {
			return k.deleteManagedClientScope(ctx, realmName, existingID, logger)
		}
		return nil
	}

	desired := buildClaimsClientScope(claims)

	if existing == nil {
		logger.Info("initial creation of client-scope", "claimsCount", len(claims))
		return k.createManagedClientScope(ctx, realmName, desired, logger)
	}

	existingCount := 0
	if existing.ProtocolMappers != nil {
		existingCount = len(*existing.ProtocolMappers)
	}
	logger.Info("configuring client-scope", "claimsCount", len(claims), "existingID", existingID, "existingCount", existingCount)

	// Ensure the scope is assigned as realm default (idempotent). This
	// covers drift where the scope exists but was manually removed from
	// realm defaults.
	if err := k.assignAsRealmDefault(ctx, realmName, existingID, logger); err != nil {
		return err
	}

	// Scope exists — reconcile individual protocol mappers.
	// Keycloak's PUT on a client scope does NOT update embedded protocol
	// mappers, so we must manage them through the sub-resource API.
	if !clientScopeMatchesDesired(existing, &desired) {
		if err := k.reconcileProtocolMappers(ctx, realmName, existingID, existing, &desired, logger); err != nil {
			return err
		}
	} else {
		logger.V(1).Info("no changes detected for managed client scope, skipping update")
	}

	return nil
}

// findManagedClientScope lists all client scopes in the realm and returns the
// managed one (if any) along with its Keycloak ID.
func (k *keycloakService) findManagedClientScope(ctx context.Context, realmName string) (*api.ClientScopeRepresentation, string, error) {
	resp, err := k.Client.GetRealmClientScopesWithResponse(ctx, realmName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list client scopes for realm %q: %w", realmName, err)
	}
	if responseErr := CheckStatusCode(resp, http.StatusOK); responseErr != nil {
		return nil, "", fmt.Errorf("listing client scopes for realm %q: %w", realmName, responseErr)
	}
	if resp.JSON2XX == nil {
		return nil, "", nil
	}
	for i, cs := range *resp.JSON2XX {
		if cs.Name != nil && *cs.Name == ManagedClientScopeName {
			if cs.Id == nil || *cs.Id == "" {
				return nil, "", fmt.Errorf("managed client scope %q exists in realm %q but has no ID", ManagedClientScopeName, realmName)
			}
			return &(*resp.JSON2XX)[i], *cs.Id, nil
		}
	}
	return nil, "", nil
}

// buildClaimsClientScope constructs a ClientScopeRepresentation with one
// protocol mapper per ClaimConfig entry. The mapper type depends on the
// claim's Type field.
func buildClaimsClientScope(claims []identityv1.ClaimConfig) api.ClientScopeRepresentation {
	mappers := make([]api.ProtocolMapperRepresentation, 0, len(claims))
	for _, c := range claims {
		mappers = append(mappers, buildProtocolMapper(c))
	}
	return api.ClientScopeRepresentation{
		Name:            ptr.To(ManagedClientScopeName),
		Description:     ptr.To("Managed by controlplane: claims for all clients"),
		Protocol:        ptr.To("openid-connect"),
		ProtocolMappers: &mappers,
	}
}

// buildProtocolMapper creates the appropriate protocol mapper representation
// for the given claim config.
func buildProtocolMapper(c identityv1.ClaimConfig) api.ProtocolMapperRepresentation {
	switch c.Type {
	case identityv1.ClaimTypeSessionNote:
		noteKey := c.Value
		if noteKey == "" {
			noteKey = c.Name
		}
		return protocolmappers.NewSessionNoteMapper(c.Name, noteKey)
	default:
		return protocolmappers.NewHardcodedClaimMapper(c.Name, c.Value)
	}
}

// clientScopeMatchesDesired compares protocol mappers between existing and
// desired representations. Only mapper name, protocolMapper type, and config
// are compared — IDs are ignored since Keycloak assigns them.
func clientScopeMatchesDesired(existing, desired *api.ClientScopeRepresentation) bool {
	existingMappers := existing.ProtocolMappers
	desiredMappers := desired.ProtocolMappers

	if existingMappers == nil && desiredMappers == nil {
		return true
	}
	if existingMappers == nil || desiredMappers == nil {
		return false
	}
	if len(*existingMappers) != len(*desiredMappers) {
		return false
	}

	// Build a map of desired mappers keyed by name for O(n) comparison.
	desiredByName := make(map[string]*api.ProtocolMapperRepresentation, len(*desiredMappers))
	for i := range *desiredMappers {
		m := &(*desiredMappers)[i]
		if m.Name != nil {
			desiredByName[*m.Name] = m
		}
	}

	for _, em := range *existingMappers {
		if em.Name == nil {
			return false
		}
		dm, ok := desiredByName[*em.Name]
		if !ok {
			return false
		}
		// Compare mapper type (e.g. hardcoded-claim vs session-note).
		if !ptrStringEqual(em.ProtocolMapper, dm.ProtocolMapper) {
			return false
		}
		switch {
		case em.Config == nil || dm.Config == nil:
			if em.Config != dm.Config {
				return false
			}
		default:
			if !protocolMapperConfigEqual(*em.Config, *dm.Config) {
				return false
			}
		}
	}
	return true
}

// ptrStringEqual compares two *string values.
func ptrStringEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// protocolMapperConfigEqual compares two protocol mapper config maps.
func protocolMapperConfigEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", va) != fmt.Sprintf("%v", vb) {
			return false
		}
	}
	return true
}

// reconcileProtocolMappers diffs existing vs desired protocol mappers and
// applies granular create/update/delete operations. This avoids removing the
// client scope (which would cause a window where tokens lack the claims).
//
//nolint:gocyclo // The reconciliation flow keeps create, update, and delete decisions in one place.
func (k *keycloakService) reconcileProtocolMappers(
	ctx context.Context,
	realmName, scopeID string,
	existing, desired *api.ClientScopeRepresentation,
	logger logr.Logger,
) error {
	// Index existing mappers by name, capturing their Keycloak-assigned IDs.
	type existingMapper struct {
		id     string
		mapper api.ProtocolMapperRepresentation
	}
	existingByName := make(map[string]existingMapper)
	if existing.ProtocolMappers != nil {
		for _, m := range *existing.ProtocolMappers {
			if m.Name != nil {
				id := ""
				if m.Id != nil {
					id = *m.Id
				}
				existingByName[*m.Name] = existingMapper{id: id, mapper: m}
			}
		}
	}

	// Index desired mappers by name.
	desiredByName := make(map[string]api.ProtocolMapperRepresentation)
	if desired.ProtocolMappers != nil {
		for _, m := range *desired.ProtocolMappers {
			if m.Name != nil {
				desiredByName[*m.Name] = m
			}
		}
	}

	// Delete mappers that are no longer desired.
	for name, em := range existingByName {
		if _, wanted := desiredByName[name]; !wanted {
			logger.Info("deleting protocol mapper", "mapper", name, "mapperId", em.id)
			resp, err := k.Client.DeleteRealmClientScopesId1ProtocolMappersModelsId2WithResponse(ctx, realmName, scopeID, em.id)
			if err != nil {
				return fmt.Errorf("failed to delete protocol mapper %q from scope %q: %w", name, scopeID, err)
			}
			if responseErr := CheckStatusCode(resp, http.StatusNoContent); responseErr != nil {
				return fmt.Errorf("deleting protocol mapper %q from scope %q: %w", name, scopeID, responseErr)
			}
		}
	}

	// Create or update desired mappers.
	for name, dm := range desiredByName {
		em, exists := existingByName[name]
		if !exists {
			// New mapper — create it.
			logger.Info("creating protocol mapper", "mapper", name)
			resp, err := k.Client.PostRealmClientScopesIdProtocolMappersModelsWithResponse(ctx, realmName, scopeID, dm)
			if err != nil {
				return fmt.Errorf("failed to create protocol mapper %q in scope %q: %w", name, scopeID, err)
			}
			if responseErr := CheckStatusCode(resp, http.StatusCreated); responseErr != nil {
				return fmt.Errorf("creating protocol mapper %q in scope %q: %w", name, scopeID, responseErr)
			}
			continue
		}

		// Existing mapper — update only if type or config changed.
		configsEqual := em.mapper.Config == nil && dm.Config == nil
		if em.mapper.Config != nil && dm.Config != nil {
			configsEqual = protocolMapperConfigEqual(*em.mapper.Config, *dm.Config)
		}
		if ptrStringEqual(em.mapper.ProtocolMapper, dm.ProtocolMapper) && configsEqual {
			continue
		}

		logger.Info("updating protocol mapper", "mapper", name, "mapperId", em.id)
		dm.Id = ptr.To(em.id)
		resp, err := k.Client.PutRealmClientScopesId1ProtocolMappersModelsId2WithResponse(ctx, realmName, scopeID, em.id, dm)
		if err != nil {
			return fmt.Errorf("failed to update protocol mapper %q in scope %q: %w", name, scopeID, err)
		}
		if responseErr := CheckStatusCode(resp, http.StatusNoContent); responseErr != nil {
			return fmt.Errorf("updating protocol mapper %q in scope %q: %w", name, scopeID, responseErr)
		}
	}

	return nil
}

// createManagedClientScope creates the scope and assigns it as a realm default.
func (k *keycloakService) createManagedClientScope(ctx context.Context, realmName string, scope api.ClientScopeRepresentation, logger logr.Logger) error {
	createResp, err := k.Client.PostRealmClientScopesWithResponse(ctx, realmName, scope)
	if err != nil {
		return fmt.Errorf("failed to create client scope %q in realm %q: %w", ManagedClientScopeName, realmName, err)
	}
	if responseErr := CheckStatusCode(createResp, http.StatusCreated); responseErr != nil {
		return fmt.Errorf("creating client scope %q in realm %q: %w", ManagedClientScopeName, realmName, responseErr)
	}

	scopeID, err := resourceIDFromResponse(createResp.HTTPResponse)
	if err != nil {
		return fmt.Errorf("client scope was created but failed to extract ID: %w", err)
	}

	logger.V(1).Info("created managed client scope", "realm", realmName, "scopeId", scopeID)

	// Assign as realm default so all clients (existing and future) inherit it.
	if err := k.assignAsRealmDefault(ctx, realmName, scopeID, logger); err != nil {
		return err
	}

	return nil
}

// deleteManagedClientScope removes the scope from realm defaults and deletes it.
func (k *keycloakService) deleteManagedClientScope(ctx context.Context, realmName, scopeID string, logger logr.Logger) error {
	// Remove from realm defaults first (ignore 404 — may not be assigned).
	delDefaultResp, err := k.Client.DeleteRealmDefaultDefaultClientScopesClientScopeIdWithResponse(ctx, realmName, scopeID)
	if err != nil {
		return fmt.Errorf("failed to remove client scope %q from realm defaults in %q: %w", ManagedClientScopeName, realmName, err)
	}
	if responseErr := CheckStatusCode(delDefaultResp, http.StatusNoContent, http.StatusNotFound); responseErr != nil {
		return fmt.Errorf("removing client scope %q from realm defaults in %q: %w", ManagedClientScopeName, realmName, responseErr)
	}

	// Delete the scope itself.
	delResp, err := k.Client.DeleteRealmClientScopesIdWithResponse(ctx, realmName, scopeID)
	if err != nil {
		return fmt.Errorf("failed to delete client scope %q in realm %q: %w", ManagedClientScopeName, realmName, err)
	}
	if responseErr := CheckStatusCode(delResp, http.StatusNoContent); responseErr != nil {
		return fmt.Errorf("deleting client scope %q in realm %q: %w", ManagedClientScopeName, realmName, responseErr)
	}

	logger.V(1).Info("deleted managed client scope", "realm", realmName, "scopeId", scopeID)
	return nil
}

// isAlreadyRealmDefault checks whether the given scope ID is already assigned as
// a realm-level default client scope.
func (k *keycloakService) isAlreadyRealmDefault(ctx context.Context, realmName, scopeID string) (bool, error) {
	resp, err := k.Client.GetRealmDefaultDefaultClientScopesWithResponse(ctx, realmName)
	if err != nil {
		return false, fmt.Errorf("listing default client scopes in realm %q: %w", realmName, err)
	}
	if responseErr := CheckStatusCode(resp, http.StatusOK); responseErr != nil {
		return false, fmt.Errorf("listing default client scopes in realm %q: %w", realmName, responseErr)
	}
	if resp.JSON2XX != nil {
		for _, s := range *resp.JSON2XX {
			if s.Id != nil && *s.Id == scopeID {
				return true, nil
			}
		}
	}
	return false, nil
}

// assignAsRealmDefault assigns the client scope as a realm-level default scope.
// It first checks whether the scope is already assigned to avoid a 409 Conflict
// from Keycloak.
func (k *keycloakService) assignAsRealmDefault(ctx context.Context, realmName, scopeID string, logger logr.Logger) error {
	alreadyDefault, err := k.isAlreadyRealmDefault(ctx, realmName, scopeID)
	if err != nil {
		return err
	}
	if alreadyDefault {
		logger.V(1).Info("client scope already assigned as realm default, skipping", "realm", realmName, "scopeId", scopeID)
		return nil
	}

	resp, err := k.Client.PutRealmDefaultDefaultClientScopesClientScopeIdWithResponse(ctx, realmName, scopeID)
	if err != nil {
		return fmt.Errorf("failed to assign client scope %q as realm default in %q: %w", ManagedClientScopeName, realmName, err)
	}
	if responseErr := CheckStatusCode(resp, http.StatusNoContent); responseErr != nil {
		return fmt.Errorf("assigning client scope %q as realm default in %q: %w", ManagedClientScopeName, realmName, responseErr)
	}

	logger.V(1).Info("assigned managed client scope as realm default", "realm", realmName, "scopeId", scopeID)
	return nil
}
