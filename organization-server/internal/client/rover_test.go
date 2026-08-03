// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
)

var testToken = accesstoken.NewStaticAccessToken("test-token")

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

	client := NewRoverClient(server.URL, testToken, "")
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

	client := NewRoverClient(server.URL, testToken, "")
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

	client := NewRoverClient(server.URL, testToken, "")
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

	client := NewRoverClient(server.URL, testToken, "")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetResources_ConnectionRefused(t *testing.T) {
	client := NewRoverClient("http://127.0.0.1:1", testToken, "")
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

	client := NewRoverClient(server.URL, testToken, "")
	_, _ = client.GetResources(context.Background(), "prod", "my-hub", "my-team")

	if capturedGroup != "my-hub" {
		t.Errorf("group: want my-hub, got %s", capturedGroup)
	}
	if capturedTeam != "my-team" {
		t.Errorf("team: want my-team, got %s", capturedTeam)
	}
}

// The internal listener authenticates with the projected SA token and derives
// its admin business context from X-Environment; both must be on the wire.
func TestGetResources_SendsProjectedTokenAndEnvironment(t *testing.T) {
	tokenFile := writeTempToken(t)

	var capturedAuth, capturedEnv string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedEnv = r.Header.Get("X-Environment")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, accesstoken.NewAccessToken(tokenFile), "")
	if _, err := client.GetResources(context.Background(), "env", "g", "t"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAuth != "Bearer "+readFile(t, tokenFile) {
		t.Errorf("Authorization: got %q", capturedAuth)
	}
	if capturedEnv != "env" {
		t.Errorf("X-Environment: want env, got %q", capturedEnv)
	}
}

// writeTempToken writes an unsigned JWT with a future exp, matching what the
// kubelet projects (the file token reader parses exp to decide on re-reads).
func writeTempToken(t *testing.T) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	tok := header + "." + base64.RawURLEncoding.EncodeToString(claims) + "."

	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(tok), 0o600); err != nil {
		t.Fatalf("writing token file: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}
