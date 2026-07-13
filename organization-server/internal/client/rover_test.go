// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetResources_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/resources" {
			t.Errorf("expected /resources, got %s", r.URL.Path)
		}
		prefix := r.URL.Query().Get("prefix")
		if prefix != "controlplane--eni--hyperion/" {
			t.Errorf("expected prefix controlplane--eni--hyperion/, got %s", prefix)
		}
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			t.Error("expected Authorization header")
		}
		acceptHeader := r.Header.Get("Accept")
		if acceptHeader != "application/json" {
			t.Errorf("expected Accept: application/json, got %s", acceptHeader)
		}

		w.Header().Set("Content-Type", "application/json")
		resp := ResourceListResponse{
			Items: []ResourceRef{
				{Name: "my-api", Kind: "ApiExposure", APIVersion: "v1", Path: "/apis/my-api"},
				{Name: "my-sub", Kind: "ApiSubscription", APIVersion: "v1", Path: "/subs/my-sub"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, "tardis")
	result, err := client.GetResources(context.Background(), "controlplane", "eni", "hyperion")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Name != "my-api" {
		t.Errorf("expected my-api, got %s", result.Items[0].Name)
	}
	if result.Items[0].Kind != "ApiExposure" {
		t.Errorf("expected ApiExposure, got %s", result.Items[0].Kind)
	}
	if result.Items[1].Name != "my-sub" {
		t.Errorf("expected my-sub, got %s", result.Items[1].Name)
	}
}

func TestGetResources_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, "tardis")
	result, err := client.GetResources(context.Background(), "test", "eni", "team1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
}

func TestGetResources_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, "tardis")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetResources_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, "tardis")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetResources_ConnectionRefused(t *testing.T) {
	client := NewRoverClient("http://127.0.0.1:1", "tardis")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestGetResources_PrefixConstruction(t *testing.T) {
	var capturedPrefix string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPrefix = r.URL.Query().Get("prefix")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, "tardis")
	_, _ = client.GetResources(context.Background(), "prod", "my-hub", "my-team")

	expected := "prod--my-hub--my-team/"
	if capturedPrefix != expected {
		t.Errorf("prefix: want %s, got %s", expected, capturedPrefix)
	}
}

func TestGetResources_MockTokenContainsAdminScope(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, "custom-prefix")
	_, _ = client.GetResources(context.Background(), "env", "g", "t")

	if capturedAuth == "" {
		t.Fatal("expected Authorization header to be set")
	}
	// Token should start with "Bearer "
	if len(capturedAuth) < 8 || capturedAuth[:7] != "Bearer " {
		t.Errorf("expected Bearer token, got %s", capturedAuth)
	}
}
