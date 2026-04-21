// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"testing"

	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak/util"
	"k8s.io/utils/ptr"
)

func TestEnsureSecretRotationPolicyEntry_UsesClientAttributesCondition(t *testing.T) {
	// Verify the condition matches what ensureSecretRotationPolicyEntry uses.
	// Note: the Keycloak provider ID is "client-attributes" (plural), not
	// "client-attribute" (singular) which is rejected with a 400.
	//
	// The configuration "attributes" value must be a JSON-encoded array of
	// {key, value} pairs because Keycloak's MapperTypeSerializer.deserialize
	// expects that format (e.g. [{"key":"foo","value":"bar"}]).
	expectedAttrs := `[{"key":"` + util.SecretRotationClientAttribute + `","value":"true"}]`
	conditions := []api.ClientPolicyConditionRepresentation{
		{
			Condition: ptr.To("client-attributes"),
			Configuration: &map[string]interface{}{
				"is.negative.logic": false,
				"attributes":        expectedAttrs,
			},
		},
	}

	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}

	cond := conditions[0]
	if *cond.Condition != "client-attributes" {
		t.Fatalf("expected condition type %q, got %q", "client-attributes", *cond.Condition)
	}

	config := *cond.Configuration
	if config["attributes"] != expectedAttrs {
		t.Fatalf("expected attributes %q, got %v", expectedAttrs, config["attributes"])
	}
	if config["is.negative.logic"] != false {
		t.Fatalf("expected is.negative.logic false, got %v", config["is.negative.logic"])
	}
}

func TestNewClientSecretRotationInfo(t *testing.T) {
	t.Run("nil cred and nil client", func(t *testing.T) {
		info := NewClientSecretRotationInfo(nil, nil)
		if info.RotatedSecret != "" {
			t.Fatal("expected empty secret")
		}
		if info.RotatedCreatedAt != nil || info.RotatedExpiresAt != nil || info.SecretCreationTime != nil {
			t.Fatal("expected nil timestamps")
		}
	})

	t.Run("with cred value and timestamps", func(t *testing.T) {
		cred := &api.CredentialRepresentation{Value: ptr.To("old-secret")}
		client := &api.ClientRepresentation{
			Attributes: &map[string]interface{}{
				attrRotatedCreationTime:   "1000",
				attrRotatedExpirationTime: float64(2000),
				attrSecretCreationTime:    "3000",
			},
		}
		info := NewClientSecretRotationInfo(cred, client)
		if info.RotatedSecret != "old-secret" {
			t.Fatalf("expected secret %q, got %q", "old-secret", info.RotatedSecret)
		}
		if info.RotatedCreatedAt == nil || *info.RotatedCreatedAt != 1000 {
			t.Fatalf("expected RotatedCreatedAt 1000, got %v", info.RotatedCreatedAt)
		}
		if info.RotatedExpiresAt == nil || *info.RotatedExpiresAt != 2000 {
			t.Fatalf("expected RotatedExpiresAt 2000, got %v", info.RotatedExpiresAt)
		}
		if info.SecretCreationTime == nil || *info.SecretCreationTime != 3000 {
			t.Fatalf("expected SecretCreationTime 3000, got %v", info.SecretCreationTime)
		}
	})
}

func TestGetSecretCreationTime(t *testing.T) {
	t.Run("nil attrs", func(t *testing.T) {
		got := GetSecretCreationTime(nil)
		if got != nil {
			t.Fatalf("expected nil, got %v", *got)
		}
	})

	t.Run("attribute present as string", func(t *testing.T) {
		attrs := map[string]interface{}{"client.secret.creation.time": "1750075200"}
		got := GetSecretCreationTime(attrs)
		if got == nil || *got != 1750075200 {
			t.Fatalf("expected 1750075200, got %v", got)
		}
	})

	t.Run("attribute present as float64", func(t *testing.T) {
		attrs := map[string]interface{}{"client.secret.creation.time": float64(1750075200)}
		got := GetSecretCreationTime(attrs)
		if got == nil || *got != 1750075200 {
			t.Fatalf("expected 1750075200, got %v", got)
		}
	})

	t.Run("attribute missing", func(t *testing.T) {
		attrs := map[string]interface{}{"other-attr": "value"}
		got := GetSecretCreationTime(attrs)
		if got != nil {
			t.Fatalf("expected nil, got %v", *got)
		}
	})
}

func TestEpochSecondsFromAttr(t *testing.T) {
	tests := []struct {
		name   string
		attrs  map[string]interface{}
		key    string
		expect *int64
	}{
		{"missing key", map[string]interface{}{}, "key", nil},
		{"nil value", map[string]interface{}{"key": nil}, "key", nil},
		{"string value", map[string]interface{}{"key": "42"}, "key", ptr.To(int64(42))},
		{"float64 value", map[string]interface{}{"key": float64(42)}, "key", ptr.To(int64(42))},
		{"invalid string", map[string]interface{}{"key": "not-a-number"}, "key", nil},
		{"bool value", map[string]interface{}{"key": true}, "key", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := epochSecondsFromAttr(tt.attrs, tt.key)
			if tt.expect == nil && got != nil {
				t.Fatalf("expected nil, got %v", *got)
			}
			if tt.expect != nil && (got == nil || *got != *tt.expect) {
				t.Fatalf("expected %v, got %v", *tt.expect, got)
			}
		})
	}
}
