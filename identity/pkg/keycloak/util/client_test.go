// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

func TestMapToClientRepresentation_SetsAttributeByDefault(t *testing.T) {
	client := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: identityv1.ClientSpec{
			ClientId:     "my-app",
			ClientSecret: "secret",
		},
	}

	rep := MapToClientRepresentation(client)

	if rep.Attributes == nil {
		t.Fatal("expected Attributes to be non-nil")
	}
	v, ok := (*rep.Attributes)[SecretRotationClientAttribute]
	if !ok {
		t.Fatalf("expected attribute %q to be present", SecretRotationClientAttribute)
	}
	if v != "true" {
		t.Fatalf("expected attribute value %q, got %v", "true", v)
	}
}

func TestClientWebOrigins(t *testing.T) {
	client := &identityv1.Client{
		Spec:   identityv1.ClientSpec{ClientId: "my-app", ClientSecret: "secret"},
		Status: identityv1.ClientStatus{AllowedOrigins: []string{"https://example.com", "*"}},
	}
	desired := MapToClientRepresentation(client)
	if desired.WebOrigins == nil || !slices.Equal(*desired.WebOrigins, client.Status.AllowedOrigins) {
		t.Fatalf("expected mapped web origins %v, got %v", client.Status.AllowedOrigins, desired.WebOrigins)
	}

	existing := MapToClientRepresentation(client)
	existing.WebOrigins = ptr.To([]string{"*", "https://example.com"})
	if CompareClientRepresentation(&existing, &desired) {
		t.Fatal("reordering web origins must cause drift")
	}
	existing.WebOrigins = ptr.To([]string{"https://other.example.com"})
	if CompareClientRepresentation(&existing, &desired) {
		t.Fatal("changed web origins must cause drift")
	}
	merged := MergeClientRepresentation(&existing, &desired)
	if merged.WebOrigins == nil || !slices.Equal(*merged.WebOrigins, *desired.WebOrigins) {
		t.Fatal("web origins were not replaced")
	}

	client.Status.AllowedOrigins = []string{}
	cleared := MapToClientRepresentation(client)
	if cleared.WebOrigins == nil || len(*cleared.WebOrigins) != 0 {
		t.Fatal("missing origins must map to an explicit empty list")
	}
	if CompareClientRepresentation(merged, &cleared) {
		t.Fatal("removing web origins must cause drift")
	}
	MergeClientRepresentation(merged, &cleared)
	if len(*merged.WebOrigins) != 0 {
		t.Fatal("web origins were not cleared")
	}
	client.Status.AllowedOrigins = nil
	empty := MapToClientRepresentation(client)
	if empty.WebOrigins == nil || len(*empty.WebOrigins) != 0 {
		t.Fatal("an omitted origins list must map to an explicit empty Keycloak list")
	}
}

func TestMapToClientRepresentation_NoAttributeWhenDisabledViaAnnotation(t *testing.T) {
	client := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				identityv1.DisableSecretRotationAnnotation: "true",
			},
		},
		Spec: identityv1.ClientSpec{
			ClientId:     "my-app",
			ClientSecret: "secret",
		},
	}

	rep := MapToClientRepresentation(client)

	if rep.Attributes != nil {
		t.Fatalf("expected Attributes to be nil, got %v", rep.Attributes)
	}
}

func TestCompareClientRepresentation_DetectsAttributeChange(t *testing.T) {
	base := &api.ClientRepresentation{
		ClientId: ptr.To("my-app"),
		Name:     ptr.To("my-app"),
		Secret:   ptr.To("secret"),
	}
	withAttr := &api.ClientRepresentation{
		ClientId: ptr.To("my-app"),
		Name:     ptr.To("my-app"),
		Secret:   ptr.To("secret"),
		Attributes: &map[string]interface{}{
			SecretRotationClientAttribute: "true",
		},
	}

	if CompareClientRepresentation(base, withAttr) {
		t.Fatal("expected representations to differ when attribute is added")
	}
	if CompareClientRepresentation(withAttr, base) {
		t.Fatal("expected representations to differ when attribute is removed")
	}
	if !CompareClientRepresentation(withAttr, withAttr) {
		t.Fatal("expected same representations to be equal")
	}
}

func TestMergeClientRepresentation_PreservesExistingAttributes(t *testing.T) {
	existing := &api.ClientRepresentation{
		ClientId: ptr.To("my-app"),
		Attributes: &map[string]interface{}{
			"some-keycloak-managed-attr": "value",
		},
	}
	desired := &api.ClientRepresentation{
		ClientId: ptr.To("my-app"),
		Attributes: &map[string]interface{}{
			SecretRotationClientAttribute: "true",
		},
	}

	merged := MergeClientRepresentation(existing, desired)

	if merged.Attributes == nil {
		t.Fatal("expected Attributes to be non-nil after merge")
	}
	attrs := *merged.Attributes
	if attrs["some-keycloak-managed-attr"] != "value" {
		t.Fatal("existing attribute was lost during merge")
	}
	if attrs[SecretRotationClientAttribute] != "true" {
		t.Fatal("new attribute was not set during merge")
	}
}

func TestMergeClientRepresentation_RemovesAttributeWhenDesiredHasNone(t *testing.T) {
	existing := &api.ClientRepresentation{
		ClientId: ptr.To("my-app"),
		Attributes: &map[string]interface{}{
			SecretRotationClientAttribute: "true",
			"other-attr":                  "keep-me",
		},
	}
	desired := &api.ClientRepresentation{
		ClientId: ptr.To("my-app"),
		// No Attributes — rotation disabled.
	}

	merged := MergeClientRepresentation(existing, desired)

	attrs := *merged.Attributes
	if _, ok := attrs[SecretRotationClientAttribute]; ok {
		t.Fatal("secret-rotation attribute should have been removed")
	}
	if attrs["other-attr"] != "keep-me" {
		t.Fatal("unrelated attribute was removed")
	}
}
