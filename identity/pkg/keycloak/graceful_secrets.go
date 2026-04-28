// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/go-logr/logr"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak/util"
	"k8s.io/utils/ptr"
)

// Constants for the managed secret-rotation profile and policy names.
const (
	secretRotationProfileName = "controlplane-secret-rotation"
	secretRotationPolicyName  = "controlplane-secret-rotation-policy"
	secretRotationExecutor    = "secret-rotation"
)

// SecretRotationParams groups the three Keycloak secret-rotation executor
// configuration values (all in seconds).
type SecretRotationParams struct {
	// ExpirationPeriodSeconds is how long a client secret is valid before it
	// must be rotated (Keycloak key: "expiration-period").
	ExpirationPeriodSeconds int

	// RotatedExpirationPeriodSeconds is how long the OLD secret remains
	// valid after rotation — the grace period (Keycloak key: "rotated-expiration-period").
	RotatedExpirationPeriodSeconds int

	// RemainingRotationPeriodSeconds is the window before expiry during
	// which rotation is allowed (Keycloak key: "remaining-rotation-period").
	RemainingRotationPeriodSeconds int
}

const (
	attrSecretCreationTime    = "client.secret.creation.time"
	attrRotatedCreationTime   = "client.secret.rotated.creation.time"
	attrRotatedExpirationTime = "client.secret.rotated.expiration.time"
)

// GetSecretCreationTime extracts the epoch-seconds timestamp of the current
// client secret from Keycloak's client attributes. Returns nil when the
// attribute is missing or cannot be parsed.
func GetSecretCreationTime(attrs map[string]interface{}) *int64 {
	if attrs == nil {
		return nil
	}
	return epochSecondsFromAttr(attrs, attrSecretCreationTime)
}

// ClientSecretRotationInfo holds the secret rotation state for a Keycloak
// client: the rotated (old) secret (if any) and the creation timestamp of
// the current secret.
type ClientSecretRotationInfo struct {
	// RotatedSecret is the plaintext value of the rotated (old) client secret.
	// Empty when no rotation is in progress.
	RotatedSecret string
	// RotatedCreatedAt is when the rotation happened (epoch seconds). Nil if unavailable.
	RotatedCreatedAt *int64
	// RotatedExpiresAt is when the rotated secret stops being accepted (epoch seconds). Nil if unavailable.
	RotatedExpiresAt *int64
	// SecretCreationTime is when the current secret was created (epoch seconds),
	// as tracked by Keycloak's secret-rotation executor. Nil if unavailable.
	SecretCreationTime *int64
}

// NewClientSecretRotationInfo builds a ClientSecretRotationInfo from the
// rotated credential response and the full client representation. The rotated
// secret value comes from cred.Value; timestamps come from client.Attributes.
func NewClientSecretRotationInfo(cred *api.CredentialRepresentation, client *api.ClientRepresentation) *ClientSecretRotationInfo {
	info := &ClientSecretRotationInfo{}
	if cred != nil && cred.Value != nil {
		info.RotatedSecret = *cred.Value
	}
	if client != nil && client.Attributes != nil {
		info.RotatedCreatedAt = epochSecondsFromAttr(*client.Attributes, attrRotatedCreationTime)
		info.RotatedExpiresAt = epochSecondsFromAttr(*client.Attributes, attrRotatedExpirationTime)
		info.SecretCreationTime = epochSecondsFromAttr(*client.Attributes, attrSecretCreationTime)
	}
	return info
}

// epochSecondsFromAttr extracts an epoch-seconds integer from a Keycloak
// attribute map. JSON deserialization into interface{} may produce either a
// string or a float64, so both are handled.
func epochSecondsFromAttr(attrs map[string]interface{}, key string) *int64 {
	v, ok := attrs[key]
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return nil
		}
		return &n
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil
		}
		n := int64(val)
		return &n
	default:
		return nil
	}
}

func (k *keycloakService) ConfigureSecretRotationPolicy(ctx context.Context, realmName string, policy *identityv1.SecretRotationConfig) error {
	logger := logr.FromContextOrDiscard(ctx)

	// ── 1. Ensure the client-policy profile exists ──────────────────────

	params := SecretRotationParams{
		ExpirationPeriodSeconds:        int(policy.ExpirationPeriod.Duration.Seconds()),
		RotatedExpirationPeriodSeconds: int(policy.GracePeriod.Duration.Seconds()),
		RemainingRotationPeriodSeconds: int(policy.RemainingRotationPeriod.Duration.Seconds()),
	}

	if err := k.ensureSecretRotationProfile(ctx, realmName, params); err != nil {
		return err
	}

	// ── 2. Ensure the client policy exists and references the profile ───

	if err := k.ensureSecretRotationPolicyEntry(ctx, realmName); err != nil {
		return err
	}

	logger.V(1).Info("secret rotation policy configured",
		"realm", realmName, "policy", policy)
	return nil
}

// ensureSecretRotationProfile creates or updates the "controlplane-secret-rotation"
// profile that carries the client-secret-rotation executor.
func (k *keycloakService) ensureSecretRotationProfile(ctx context.Context, realmName string, params SecretRotationParams) error {
	logger := logr.FromContextOrDiscard(ctx)

	getResp, err := k.Client.GetRealmClientPoliciesProfilesWithResponse(
		ctx, realmName, &api.GetRealmClientPoliciesProfilesParams{})
	if err != nil {
		return fmt.Errorf("failed to get client profiles for realm %s: %w", realmName, err)
	}
	if responseErr := CheckStatusCode(getResp, http.StatusOK); responseErr != nil {
		return fmt.Errorf("unexpected status getting client profiles for realm %s: %d: %w",
			realmName, getResp.StatusCode(), responseErr)
	}

	profiles := getResp.JSON2XX
	if profiles == nil {
		profiles = &api.ClientProfilesRepresentation{}
	}

	// Keycloak's secret-rotation executor expects kebab-case configuration keys
	// with integer values in seconds:
	//   expiration-period          – how long a client secret is valid
	//   rotated-expiration-period  – grace period for the OLD secret after rotation
	//   remaining-rotation-period  – window before expiry where rotation is allowed
	// Validation: expirationPeriod > 0, rotatedExpirationPeriod <= expirationPeriod,
	//             remainingRotationPeriod <= expirationPeriod.
	desiredExecutor := api.ClientPolicyExecutorRepresentation{
		Executor: ptr.To(secretRotationExecutor),
		Configuration: &map[string]interface{}{
			"expiration-period":         params.ExpirationPeriodSeconds,
			"rotated-expiration-period": params.RotatedExpirationPeriodSeconds,
			"remaining-rotation-period": params.RemainingRotationPeriodSeconds,
		},
	}
	desiredProfile := api.ClientProfileRepresentation{
		Name:        ptr.To(secretRotationProfileName),
		Description: ptr.To("Managed by controlplane: enables graceful client-secret rotation"),
		Executors:   &[]api.ClientPolicyExecutorRepresentation{desiredExecutor},
	}

	// Look for an existing managed profile and update in place, or append.
	found := false
	if profiles.Profiles != nil {
		for i, p := range *profiles.Profiles {
			if p.Name != nil && *p.Name == secretRotationProfileName {
				(*profiles.Profiles)[i] = desiredProfile
				found = true
				break
			}
		}
	}
	if !found {
		if profiles.Profiles == nil {
			profiles.Profiles = &[]api.ClientProfileRepresentation{}
		}
		*profiles.Profiles = append(*profiles.Profiles, desiredProfile)
	}

	// Strip read-only global profiles — Keycloak returns 400 if they are
	// included in the PUT body and differ from the server-side originals.
	profiles.GlobalProfiles = nil

	logger.V(1).Info("putting client profiles", "realm", realmName, "found", found)

	// PUT replaces the full profiles list. We preserved other profiles above.
	putResp, err := k.Client.PutRealmClientPoliciesProfilesWithResponse(ctx, realmName, *profiles)
	if err != nil {
		return fmt.Errorf("failed to put client profiles for realm %s: %w", realmName, err)
	}
	if responseErr := CheckStatusCode(putResp, http.StatusNoContent); responseErr != nil {
		return fmt.Errorf("unexpected status putting client profiles for realm %s: %d: %w",
			realmName, putResp.StatusCode(), responseErr)
	}
	return nil
}

// ensureSecretRotationPolicyEntry creates or updates the
// "controlplane-secret-rotation-policy" that references the managed profile.
func (k *keycloakService) ensureSecretRotationPolicyEntry(ctx context.Context, realmName string) error {
	logger := logr.FromContextOrDiscard(ctx)

	getResp, err := k.Client.GetRealmClientPoliciesPoliciesWithResponse(
		ctx, realmName, &api.GetRealmClientPoliciesPoliciesParams{})
	if err != nil {
		return fmt.Errorf("failed to get client policies for realm %s: %w", realmName, err)
	}
	if responseErr := CheckStatusCode(getResp, http.StatusOK); responseErr != nil {
		return fmt.Errorf("unexpected status getting client policies for realm %s: %d: %w",
			realmName, getResp.StatusCode(), responseErr)
	}

	policies := getResp.JSON2XX
	if policies == nil {
		policies = &api.ClientPoliciesRepresentation{}
	}

	desiredPolicy := api.ClientPolicyRepresentation{
		Name:        ptr.To(secretRotationPolicyName),
		Description: ptr.To("Managed by controlplane: applies secret-rotation profile to opted-in clients"),
		Enabled:     ptr.To(true),
		Profiles:    &[]string{secretRotationProfileName},
		Conditions: &[]api.ClientPolicyConditionRepresentation{
			{
				Condition: ptr.To("client-attributes"),
				Configuration: &map[string]interface{}{
					"is.negative.logic": false,
					// Keycloak's client-attributes condition deserialises the "attributes"
					// value with MapperTypeSerializer which expects a JSON-encoded array
					// of {key, value} pairs – NOT a plain map.
					"attributes": marshalPolicyAttributes(util.SecretRotationClientAttribute, "true"),
				},
			},
		},
	}

	// Look for an existing managed policy and update in place, or append.
	found := false
	if policies.Policies != nil {
		for i, p := range *policies.Policies {
			if p.Name != nil && *p.Name == secretRotationPolicyName {
				(*policies.Policies)[i] = desiredPolicy
				found = true
				break
			}
		}
	}
	if !found {
		if policies.Policies == nil {
			policies.Policies = &[]api.ClientPolicyRepresentation{}
		}
		*policies.Policies = append(*policies.Policies, desiredPolicy)
	}

	// Strip read-only global policies — same rationale as GlobalProfiles above.
	policies.GlobalPolicies = nil

	logger.V(1).Info("putting client policies", "realm", realmName, "found", found)

	putResp, err := k.Client.PutRealmClientPoliciesPoliciesWithResponse(ctx, realmName, *policies)
	if err != nil {
		return fmt.Errorf("failed to put client policies for realm %s: %w", realmName, err)
	}
	if responseErr := CheckStatusCode(putResp, http.StatusNoContent); responseErr != nil {
		return fmt.Errorf("unexpected status putting client policies for realm %s: %d: %w",
			realmName, putResp.StatusCode(), responseErr)
	}
	return nil
}

// GetClientSecretRotationInfo fetches the secret rotation state for a Keycloak
// client in a single getClient call. It always returns a non-nil
// *ClientSecretRotationInfo (when the client exists), populated with whatever
// data is available: the rotated secret (if a grace period is active) and the
// current secret's creation timestamp.
func (k *keycloakService) GetClientSecretRotationInfo(ctx context.Context, realmName string, client *identityv1.Client) (*ClientSecretRotationInfo, error) {
	logger := logr.FromContextOrDiscard(ctx)

	// Resolve the Keycloak-internal UUID for this client.
	existing, err := k.getClient(ctx, realmName, client)
	if err != nil {
		return nil, fmt.Errorf("failed to look up client %q: %w", client.Spec.ClientId, err)
	}
	if existing == nil || existing.Id == nil || *existing.Id == "" {
		return nil, fmt.Errorf("client %q not found in Keycloak", client.Spec.ClientId)
	}
	keycloakId := *existing.Id

	// Start with creation time from the existing client representation.
	info := &ClientSecretRotationInfo{}
	if existing.Attributes != nil {
		info.SecretCreationTime = epochSecondsFromAttr(*existing.Attributes, attrSecretCreationTime)
	}

	// Check for a rotated (old) secret.
	resp, err := k.Client.GetRealmClientsIdClientSecretRotatedWithResponse(ctx, realmName, keycloakId)
	if err != nil {
		return nil, fmt.Errorf("failed to get rotated secret for client %q: %w", client.Spec.ClientId, err)
	}

	logger.V(1).Info("received response for rotated secret check", "clientId", client.Spec.ClientId, "status", resp.StatusCode())

	// 404 means no rotated secret exists — rotation not in progress or grace period expired.
	if resp.StatusCode() == http.StatusNotFound {
		logger.V(1).Info("no rotated secret found", "clientId", client.Spec.ClientId)
		return info, nil
	}

	if responseErr := CheckStatusCode(resp, http.StatusOK); responseErr != nil {
		return nil, fmt.Errorf("unexpected status getting rotated secret for client %s: %d: %w",
			client.Spec.ClientId, resp.StatusCode(), responseErr)
	}

	if resp.JSON2XX == nil || resp.JSON2XX.Value == nil || *resp.JSON2XX.Value == "" {
		logger.V(1).Info("rotated secret response has no value", "clientId", client.Spec.ClientId)
		return info, nil
	}

	logger.V(1).Info("rotated secret found", "clientId", client.Spec.ClientId)
	rotated := NewClientSecretRotationInfo(resp.JSON2XX, existing)
	// Preserve SecretCreationTime already extracted above.
	rotated.SecretCreationTime = info.SecretCreationTime
	return rotated, nil
}

// marshalPolicyAttributes produces the JSON-encoded array of {key, value} pairs
// expected by Keycloak's client-attributes condition (MapperTypeSerializer).
func marshalPolicyAttributes(key, value string) string {
	type kvPair struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	b, _ := json.Marshal([]kvPair{{Key: key, Value: value}})
	return string(b)
}

// forceSecretRotation triggers Keycloak's secret-rotation executor by POSTing
// to /clients/{id}/client-secret. This moves the current secret into the
// "rotated" slot with the configured grace period and generates a random new
// secret (which the caller typically overwrites with a PUT immediately after).
func (k *keycloakService) forceSecretRotation(ctx context.Context, realmName, keycloakId string) error {
	log := logr.FromContextOrDiscard(ctx)

	delRes, err := k.Client.DeleteRealmClientsIdClientSecretRotatedWithResponse(ctx, realmName, keycloakId) // best-effort cleanup of any existing rotated secret
	if err != nil {
		log.V(1).Info("failed to delete existing rotated secret (ignoring)", "realm", realmName, "clientId", keycloakId, "error", err)
	}
	if delRes != nil && delRes.StatusCode() != http.StatusNoContent && delRes.StatusCode() != http.StatusNotFound {
		log.V(1).Info("unexpected status deleting existing rotated secret (ignoring)", "realm", realmName, "clientId", keycloakId, "status", delRes.StatusCode())
	} else {
		log.V(1).Info("existing rotated secret deleted or not found, proceeding with rotation", "realm", realmName, "clientId", keycloakId)
	}

	res, err := k.Client.PostRealmClientsIdClientSecretWithResponse(ctx, realmName, keycloakId)
	if err != nil {
		return fmt.Errorf("error forcing secret rotation: %w", err)
	}
	if responseErr := CheckStatusCode(res, http.StatusOK); responseErr != nil {
		return fmt.Errorf("forcing secret rotation: %w", responseErr)
	}
	log.V(1).Info("forced client secret rotation in keycloak", "realm", realmName, "keycloakId", keycloakId)

	return nil
}
