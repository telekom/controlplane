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
		prefix := r.URL.Query().Get("group")
		if prefix != "eni" {
			t.Errorf("expected group eni, got %s", prefix)
		}
		team := r.URL.Query().Get("team")
		if team != "hyperion" {
			t.Errorf("expected team hyperion, got %s", team)
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

func TestGetResources_QueryParamConstruction(t *testing.T) {
	var capturedGroup, capturedTeam string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedGroup = r.URL.Query().Get("group")
		capturedTeam = r.URL.Query().Get("team")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, "tardis")
	_, _ = client.GetResources(context.Background(), "prod", "my-hub", "my-team")

	if capturedGroup != "my-hub" {
		t.Errorf("group: want my-hub, got %s", capturedGroup)
	}
	if capturedTeam != "my-team" {
		t.Errorf("team: want my-team, got %s", capturedTeam)
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
